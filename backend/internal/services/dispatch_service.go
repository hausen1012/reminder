// dispatch_service 是一次触发的"渲染 + 通道并发发送 + 重试 + 落 attempts 与汇总状态"链条。
//
// 与 Engine 的契约：Engine 在事务内插入一条 status=pending 的 delivery_log，
// 随后异步调 Run(reminder, deliveryLogID)；本服务负责把这条 log 推进到终态。
package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/bedrock/backend/internal/models"
	"github.com/bedrock/backend/internal/notifier"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// DispatchService 负责一次触发的实际投递。
type DispatchService struct {
	DB         *gorm.DB
	ChannelSvc *ChannelService
	Loc        *time.Location

	// 确认机制（可选）；为空时跳过确认逻辑
	ConfirmMgr *ConfirmRetryManager

	// 单通道串行重试间隔；空时使用默认 [0, 10s, 30s]
	RetryDelays []time.Duration
}

// NewDispatchService 构造 dispatch 服务。
func NewDispatchService(db *gorm.DB, channelSvc *ChannelService, loc *time.Location) *DispatchService {
	return &DispatchService{
		DB:          db,
		ChannelSvc:  channelSvc,
		Loc:         loc,
		RetryDelays: []time.Duration{0, 10 * time.Second, 30 * time.Second},
	}
}

// Run 是 Engine.fire 异步调用的入口。
//
// 流程：
//  1. 加载该提醒绑定的所有 enabled 通道（无绑定 → 标 failed）
//  2. 构造 vars（不含 confirm_url；Phase 5 接入确认机制后注入）
//  3. 渲染 title / content（一次性）；为每个通道执行 send-with-retry
//  4. 汇总 attempts 状态 → 写回 delivery_log.status 与渲染后的 title/content
func (d *DispatchService) Run(ctx context.Context, r *models.Reminder, deliveryLogID uint) {
	if r == nil || deliveryLogID == 0 {
		return
	}
	channels, err := d.loadChannels(r.ID)
	if err != nil {
		log.Printf("[dispatch] 加载通道失败 reminder=%d: %v", r.ID, err)
		d.finalize(deliveryLogID, "failed", r.Title, r.Content)
		return
	}
	if len(channels) == 0 {
		log.Printf("[dispatch] reminder=%d 没有可用通道", r.ID)
		d.finalize(deliveryLogID, "failed", r.Title, r.Content)
		return
	}

	planned := r.NextFireAt
	if planned == nil {
		planned = ptrTime(time.Now())
	}
	vars := buildVars(r, time.Now(), *planned, d.Loc)

	// 需要确认时，首发生成 chain_id + token；重发复用已有 chain_id + token
	var confirmChainID string
	if r.RequireConfirm && d.ConfirmMgr != nil {
		var currentLog models.DeliveryLog
		if err := d.DB.First(&currentLog, deliveryLogID).Error; err != nil {
			log.Printf("[dispatch] 加载确认日志失败 log=%d: %v", deliveryLogID, err)
		} else if currentLog.ConfirmChainID != nil && *currentLog.ConfirmChainID != "" {
			confirmChainID = *currentLog.ConfirmChainID
			var tok models.ConfirmToken
			err := d.DB.Joins("JOIN delivery_logs ON delivery_logs.id = confirm_tokens.delivery_log_id").
				Where("delivery_logs.confirm_chain_id = ?", confirmChainID).
				First(&tok).Error
			if err != nil {
				log.Printf("[dispatch] 复用确认 token 失败 chain=%s log=%d: %v", confirmChainID, deliveryLogID, err)
			} else {
				vars["confirm_url"] = d.ConfirmMgr.ConfirmSvc.BuildURL(tok.Token)
			}
		} else {
			confirmChainID = uuid.New().String()
			ttl := 72 * time.Hour // 充足有效期
			token, err := d.ConfirmMgr.ConfirmSvc.CreateToken(deliveryLogID, ttl)
			if err == nil {
				vars["confirm_url"] = d.ConfirmMgr.ConfirmSvc.BuildURL(token)
				d.DB.Model(&models.DeliveryLog{}).Where("id = ?", deliveryLogID).
					Update("confirm_chain_id", confirmChainID)
				log.Printf("[confirm] 创建确认链 chain=%s reminder=%d log=%d", confirmChainID, r.ID, deliveryLogID)
			} else {
				log.Printf("[dispatch] 创建确认 token 失败 log=%d: %v", deliveryLogID, err)
				confirmChainID = ""
			}
		}
	}

	body := notifier.Render(r.Content, vars)
	if confirmChainID != "" && !strings.Contains(r.Content, "{{confirm_url}}") {
		cu, _ := vars["confirm_url"]
		switch r.ContentFormat {
		case "html":
			body += `<br><a href="` + cu + `">点击确认链接</a>`
		case "markdown":
			body += "\n\n[点击确认提醒](" + cu + ")"
		default:
			body += "\n点击确认提醒：" + cu
		}
	}
	rendered := notifier.Message{
		Subject: notifier.Render(r.Title, vars),
		Body:    body,
		Format:  r.ContentFormat,
		Vars:    vars,
	}

	type result struct {
		channelID   uint
		channelType string
		channelName string
		ok          bool
	}

	var wg sync.WaitGroup
	results := make([]result, len(channels))
	for i := range channels {
		i := i
		ch := channels[i]
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok := d.sendWithRetry(ctx, ch, deliveryLogID, rendered)
			results[i] = result{
				channelID:   ch.ID,
				channelType: ch.Type,
				channelName: ch.Name,
				ok:          ok,
			}
		}()
	}
	wg.Wait()

	successCnt, failCnt := 0, 0
	for _, r := range results {
		if r.ok {
			successCnt++
		} else {
			failCnt++
		}
	}
	status := "success"
	switch {
	case successCnt == 0:
		status = "failed"
	case failCnt > 0:
		status = "partial"
	}
	d.finalize(deliveryLogID, status, rendered.Subject, rendered.Body)

	// 仅首发在成功进入确认链后调度重发；重发轮次由 ConfirmRetryManager 继续串联
	if confirmChainID != "" && d.ConfirmMgr != nil {
		var currentLog models.DeliveryLog
		if err := d.DB.First(&currentLog, deliveryLogID).Error; err != nil {
			log.Printf("[confirm] 读取当前日志失败 log=%d: %v", deliveryLogID, err)
		} else if currentLog.RetryRound != 0 {
			log.Printf("[confirm] 跳过首发调度 chain=%s：当前为重发轮次 %d", confirmChainID, currentLog.RetryRound)
		} else {
			var refreshed models.Reminder
			if err := d.DB.First(&refreshed, r.ID).Error; err != nil {
				log.Printf("[confirm] 重读 reminder 失败 reminder=%d: %v", r.ID, err)
			} else {
				log.Printf("[confirm] 调度确认重发 chain=%s reminder=%d interval=%ds max_retries=%d", confirmChainID, refreshed.ID, refreshed.ConfirmRetryIntervalSec, refreshed.ConfirmMaxRetries)
				d.ConfirmMgr.Schedule(&refreshed, confirmChainID, 0)
			}
		}
	}
}

// sendWithRetry 在单个通道上按 RetryDelays 串行重试，写入 DeliveryAttempt。
//
// 永久错误（ErrPermanent）立即停止；其他错误等下一个 delay 再试。
// 返回值：是否最终成功。
func (d *DispatchService) sendWithRetry(ctx context.Context, ch *models.Channel, deliveryLogID uint, msg notifier.Message) bool {
	n, err := notifier.Get(ch.Type)
	if err != nil {
		log.Printf("[dispatch] 获取通道发送器失败 log=%d channel=%d name=%s type=%s: %v", deliveryLogID, ch.ID, ch.Name, ch.Type, err)
		d.writeAttempt(deliveryLogID, ch, 1, "failed", err.Error(), 0)
		return false
	}
	plainConfig, err := d.ChannelSvc.DecryptedConfig(ch)
	if err != nil {
		log.Printf("[dispatch] 解密通道配置失败 log=%d channel=%d name=%s type=%s: %v", deliveryLogID, ch.ID, ch.Name, ch.Type, err)
		d.writeAttempt(deliveryLogID, ch, 1, "failed", "decrypt config: "+err.Error(), 0)
		return false
	}

	for i, delay := range d.RetryDelays {
		if delay > 0 {
			select {
			case <-ctx.Done():
				return false
			case <-time.After(delay):
			}
		}
		started := time.Now()
		err := n.Send(ctx, plainConfig, msg)
		latency := int(time.Since(started).Milliseconds())
		if err == nil {
			d.writeAttempt(deliveryLogID, ch, i+1, "success", "", latency)
			return true
		}
		log.Printf("[dispatch] 通道发送失败 log=%d channel=%d name=%s type=%s attempt=%d latency=%dms: %v", deliveryLogID, ch.ID, ch.Name, ch.Type, i+1, latency, err)
			d.writeAttempt(deliveryLogID, ch, i+1, "failed", err.Error(), latency)
		if notifier.IsPermanent(err) {
			return false
		}
	}
	return false
}

// writeAttempt 落一条 DeliveryAttempt。
func (d *DispatchService) writeAttempt(deliveryLogID uint, ch *models.Channel, attempt int, status, errMsg string, latencyMs int) {
	if len(errMsg) > 1000 {
		errMsg = errMsg[:1000]
	}
	row := &models.DeliveryAttempt{
		DeliveryLogID: deliveryLogID,
		ChannelID:     ch.ID,
		ChannelType:   ch.Type,
		ChannelName:   ch.Name,
		Attempt:       attempt,
		Status:        status,
		Error:         errMsg,
		LatencyMs:     latencyMs,
	}
	if err := d.DB.Create(row).Error; err != nil {
		log.Printf("[dispatch] 写 attempt 失败 log=%d ch=%d: %v", deliveryLogID, ch.ID, err)
	}
}

// finalize 更新 DeliveryLog 的终态字段。
func (d *DispatchService) finalize(deliveryLogID uint, status, renderedTitle, renderedContent string) {
	updates := map[string]any{
		"status":  status,
		"title":   renderedTitle,
		"content": renderedContent,
	}
	if err := d.DB.Model(&models.DeliveryLog{}).Where("id = ?", deliveryLogID).Updates(updates).Error; err != nil {
		log.Printf("[dispatch] finalize 失败 log=%d: %v", deliveryLogID, err)
	}
}

// loadChannels 取 reminder 绑定的全部 enabled 通道。
//
// 关联表是 ReminderChannel，且只选择 enabled=true 的通道；
// 没有绑定或全部禁用时返回空切片。
func (d *DispatchService) loadChannels(reminderID uint) ([]*models.Channel, error) {
	var ids []uint
	if err := d.DB.Model(&models.ReminderChannel{}).
		Where("reminder_id = ?", reminderID).
		Pluck("channel_id", &ids).Error; err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}
	var rows []models.Channel
	if err := d.DB.Where("id IN ? AND enabled = ?", ids, true).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]*models.Channel, 0, len(rows))
	for i := range rows {
		out = append(out, &rows[i])
	}
	return out, nil
}

// DryRun 发送测试消息，不落任何 delivery_log / delivery_attempt。
func (d *DispatchService) DryRun(ctx context.Context, channels []*models.Channel, msg notifier.Message) error {
	var wg sync.WaitGroup
	errCh := make(chan error, len(channels))
	for i := range channels {
		i := i
		ch := channels[i]
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := d.sendDryRun(ctx, ch, msg); err != nil {
				errCh <- fmt.Errorf("channel=%d name=%s type=%s: %w", ch.ID, ch.Name, ch.Type, err)
			}
		}()
	}
	wg.Wait()
	close(errCh)
	// 收集所有错误
	var errs []string
	for err := range errCh {
		errs = append(errs, err.Error())
	}
	if len(errs) > 0 {
		return fmt.Errorf("试发失败: %s", strings.Join(errs, "; "))
	}
	return nil
}

// sendDryRun 单通道试发（有重试，不写 attempt）。
func (d *DispatchService) sendDryRun(ctx context.Context, ch *models.Channel, msg notifier.Message) error {
	n, err := notifier.Get(ch.Type)
	if err != nil {
		return err
	}
	plainConfig, err := d.ChannelSvc.DecryptedConfig(ch)
	if err != nil {
		return fmt.Errorf("解密配置失败: %w", err)
	}
	for i, delay := range d.RetryDelays {
		if delay > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}
		if sendErr := n.Send(ctx, plainConfig, msg); sendErr == nil {
			return nil
		} else {
			log.Printf("[dispatch-dryrun] 通道发送失败 ch=%d name=%s attempt=%d: %v", ch.ID, ch.Name, i+1, sendErr)
			if notifier.IsPermanent(sendErr) {
				return sendErr
			}
		}
	}
	return fmt.Errorf("所有重试均失败")
}

// --- helpers ---

func ptrTime(t time.Time) *time.Time { return &t }

// debugVars 仅供测试观察渲染结果。
func debugVars(vars map[string]string) string {
	b, _ := json.Marshal(vars)
	return string(b)
}

// 显式引用以避免 "imported and not used" 的潜在告警。
var _ = strings.TrimSpace
var _ = errors.New
var _ = fmt.Sprintf
var _ = debugVars
