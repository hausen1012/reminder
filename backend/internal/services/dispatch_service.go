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
	"gorm.io/gorm"
)

// DispatchService 负责一次触发的实际投递。
type DispatchService struct {
	DB         *gorm.DB
	ChannelSvc *ChannelService
	Loc        *time.Location

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
	rendered := notifier.Message{
		Subject: notifier.Render(r.Title, vars),
		Body:    notifier.Render(r.Content, vars),
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
}

// sendWithRetry 在单个通道上按 RetryDelays 串行重试，写入 DeliveryAttempt。
//
// 永久错误（ErrPermanent）立即停止；其他错误等下一个 delay 再试。
// 返回值：是否最终成功。
func (d *DispatchService) sendWithRetry(ctx context.Context, ch *models.Channel, deliveryLogID uint, msg notifier.Message) bool {
	n, err := notifier.Get(ch.Type)
	if err != nil {
		d.writeAttempt(deliveryLogID, ch, 1, "failed", err.Error(), 0)
		return false
	}
	plainConfig, err := d.ChannelSvc.DecryptedConfig(ch)
	if err != nil {
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

// TestOnce 立刻跑一次 dispatch（用于 /reminders/:id/test）。
//
// 写一条 status=pending 的日志再 Run，等同正常触发但绕过调度。
func (d *DispatchService) TestOnce(ctx context.Context, r *models.Reminder) (uint, error) {
	now := time.Now()
	dlog := &models.DeliveryLog{
		ReminderID: r.ID,
		FiredAt:    now,
		Title:      r.Title,
		Content:    r.Content,
		Status:     "pending",
		Source:     r.Source,
		RetryRound: 0,
	}
	if err := d.DB.Create(dlog).Error; err != nil {
		return 0, err
	}
	d.Run(ctx, r, dlog.ID)
	return dlog.ID, nil
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
