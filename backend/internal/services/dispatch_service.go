// dispatch_service 负责提醒消息渲染、通道并发投递、重试及日志状态更新。
package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/reminder/backend/internal/middleware"
	"github.com/reminder/backend/internal/models"
	"github.com/reminder/backend/internal/notifier"
	"github.com/reminder/backend/internal/textutil"
	"github.com/reminder/backend/internal/timezone"
	"gorm.io/gorm"
)

// DispatchService 负责一次触发的实际投递。
type DispatchService struct {
	DB         *gorm.DB
	ChannelSvc *ChannelService
	TZ         *timezone.Provider

	// 确认机制（可选）；为空时跳过确认逻辑
	ConfirmMgr *ConfirmRetryManager

	// 单通道串行重试间隔；空时使用默认 [0, 10s, 30s]
	RetryDelays []time.Duration
}

// NewDispatchService 构造 dispatch 服务。
func NewDispatchService(db *gorm.DB, channelSvc *ChannelService, tz *timezone.Provider) *DispatchService {
	if tz == nil {
		tz = timezone.New("")
	}
	return &DispatchService{
		DB:          db,
		ChannelSvc:  channelSvc,
		TZ:          tz,
		RetryDelays: []time.Duration{0, 10 * time.Second, 30 * time.Second},
	}
}

// Run 是 Engine.fire 异步调用的入口：加载通道 → 构建变量并渲染 → 各通道并发发送并重试
// → 汇总结果写回日志状态。
func (d *DispatchService) Run(ctx context.Context, r *models.Reminder, deliveryLogID uint) {
	if r == nil || deliveryLogID == 0 {
		return
	}
	started := time.Now()
	slog.Info("开始投递", "reminder", r.ID, "log", deliveryLogID, "title", r.Title, "require_confirm", r.RequireConfirm)
	channels, err := d.loadChannels(r.ID)
	if err != nil {
		slog.Error("加载提醒绑定的通道失败，本次投递标记为 failed", "reminder", r.ID, "log", deliveryLogID, "error", err)
		d.finalize(deliveryLogID, "failed", r.Title, r.Content)
		return
	}
	if len(channels) == 0 {
		slog.Warn("提醒没有可用的通道（未绑定或全部被禁用），本次投递标记为 failed", "reminder", r.ID, "log", deliveryLogID)
		d.finalize(deliveryLogID, "failed", r.Title, r.Content)
		return
	}
	chNames := make([]string, len(channels))
	for i, ch := range channels {
		chNames[i] = ch.Name + "(" + ch.Type + ")"
	}
	// 中间步骤都用 Debug，真正需要看的是最后那条汇总与出错分支
	slog.Debug("已加载通道", "reminder", r.ID, "channels", chNames)

	planned := r.NextFireAt
	if planned == nil {
		fresh := models.TimeNow()
		planned = &fresh
	}
	vars := buildVars(r, time.Now(), planned.Std(), d.TZ.Get())
	// 不打印 vars 内容：confirm_url 带确认令牌，记长度即可
	slog.Debug("变量构建完成", "reminder", r.ID, "vars_count", len(vars))

	// 需要确认时，首发生成 chain_id + token；重发复用已有 chain_id + token
	var confirmChainID string
	if r.RequireConfirm && d.ConfirmMgr != nil {
		slog.Debug("提醒需要确认", "reminder", r.ID)
		var currentLog models.DeliveryLog
		if err := d.DB.First(&currentLog, deliveryLogID).Error; err != nil {
			slog.Warn("加载确认日志失败，无法复用已有确认链", "log", deliveryLogID, "error", err)
		} else if currentLog.ConfirmChainID != nil && *currentLog.ConfirmChainID != "" {
			confirmChainID = *currentLog.ConfirmChainID
			tok, err := tokenByChain(d.DB, confirmChainID)
			if err != nil {
				slog.Warn("复用确认 token 失败，本轮将不带确认链接", "chain", confirmChainID, "log", deliveryLogID, "error", err)
			} else {
				vars["confirm_url"] = d.ConfirmMgr.ConfirmSvc.BuildURL(tok.Token)
			}
		} else {
			// 新触发：取消该提醒此前的确认重发链
			if d.ConfirmMgr != nil {
				d.ConfirmMgr.CancelByReminderID(r.ID)
			}
			confirmChainID = uuid.New().String()
			token, err := d.ConfirmMgr.ConfirmSvc.CreateToken(deliveryLogID, ConfirmTokenTTL)
			if err == nil {
				vars["confirm_url"] = d.ConfirmMgr.ConfirmSvc.BuildURL(token)
				d.DB.Model(&models.DeliveryLog{}).Where("id = ?", deliveryLogID).
					Update("confirm_chain_id", confirmChainID)
				slog.Debug("创建确认链", "chain", confirmChainID, "reminder", r.ID, "log", deliveryLogID)
			} else {
				slog.Error("创建确认 token 失败，本次发送将不含确认链接", "reminder", r.ID, "log", deliveryLogID, "error", err)
				confirmChainID = ""
			}
		}
	}

	// 先注入 confirm_url，再统一渲染标题与正文；正文渲染后按格式补上确认链接
	rendered := buildRenderedMessageWith(r.Title, r.Content, r.ContentFormat, vars, func(body string) string {
		next, appended := appendConfirmLink(body, vars["confirm_url"], r.ContentFormat)
		if appended {
			slog.Debug("自动追加确认链接", "reminder", r.ID)
		}
		return next
	})
	slog.Debug("内容渲染完成", "reminder", r.ID, "body_len", len(rendered.Body), "confirm", confirmChainID != "")
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
			defer func() {
				if r := recover(); r != nil {
					slog.Error("投递协程 panic", "recover", r, "log", deliveryLogID)
				}
			}()
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
	// 一次投递只留这一条汇总，通道级明细在 Debug 与投递日志页里
	slog.Info("投递完成", "reminder", r.ID, "log", deliveryLogID, "title", r.Title, "status", status,
		"success", successCnt, "failed", failCnt, "total", len(channels),
		"elapsed_ms", time.Since(started).Milliseconds())
	d.finalize(deliveryLogID, status, rendered.Subject, rendered.Body)

	// 仅首发在成功进入确认链后调度重发；重发轮次由 ConfirmRetryManager 继续串联
	if confirmChainID != "" && d.ConfirmMgr != nil {
		var currentLog models.DeliveryLog
		if err := d.DB.First(&currentLog, deliveryLogID).Error; err != nil {
			slog.Warn("读取当前日志失败，跳过首发调度", "log", deliveryLogID, "error", err)
		} else if currentLog.RetryRound != 0 {
			slog.Debug("跳过首发调度", "chain", confirmChainID, "retry_round", currentLog.RetryRound)
		} else {
			var refreshed models.Reminder
			if err := d.DB.First(&refreshed, r.ID).Error; err != nil {
				slog.Error("重读提醒失败，无法调度确认重发", "reminder", r.ID, "chain", confirmChainID, "error", err)
			} else {
				slog.Debug("调度确认重发", "chain", confirmChainID, "reminder", refreshed.ID, "interval_sec", refreshed.ConfirmRetryIntervalSec, "max_retries", refreshed.ConfirmMaxRetries)
				d.ConfirmMgr.Schedule(&refreshed, confirmChainID, 0)
			}
		}
	}
}

// sendPlan 是单通道发送前的准备结果：通知器、按通道上限截断后的消息与解密后的配置。
type sendPlan struct {
	notifier notifier.Notifier
	msg      notifier.Message
	config   []byte
}

// sendAttempt 是一次尝试的记录回调；为 nil 时不落库（试发）。
type sendAttempt func(attempt int, status, errMsg string, latencyMs int)

// prepare 解析通道类型、按该通道上限截断正文并解密配置。
func (d *DispatchService) prepare(ch *models.Channel, msg notifier.Message) (*sendPlan, error) {
	n, err := notifier.Get(ch.Type)
	if err != nil {
		return nil, err
	}
	return d.buildPlan(ch, n, msg)
}

// buildPlan 用已解析的通知器构造发送计划（截断正文 + 解密配置）。
func (d *DispatchService) buildPlan(ch *models.Channel, n notifier.Notifier, msg notifier.Message) (*sendPlan, error) {
	plainConfig, err := d.ChannelSvc.DecryptedConfig(ch)
	if err != nil {
		return nil, err
	}
	return &sendPlan{
		notifier: n,
		msg:      notifier.LimitBody(ch.Type, msg),
		config:   plainConfig,
	}, nil
}

// runRetries 在单个通道上按 RetryDelays 串行发送；永久错误立即停止。
//
// stage 只用于日志文案（「发送」/「试发」），record 为 nil 时不落 attempt。
// 返回 nil 表示某次尝试成功，否则返回最后一次错误。
func (d *DispatchService) runRetries(ctx context.Context, ch *models.Channel, plan *sendPlan, stage string, record sendAttempt) error {
	var lastErr error
	for i, delay := range d.RetryDelays {
		if delay > 0 {
			select {
			case <-ctx.Done():
				slog.Warn("投递上下文已取消，停止重试", "channel", ch.ID, "attempt", i+1, "error", ctx.Err())
				return ctx.Err()
			case <-time.After(delay):
			}
		}
		started := time.Now()
		err := plan.notifier.Send(ctx, plan.config, plan.msg)
		latency := int(time.Since(started).Milliseconds())
		if err == nil {
			// 单次成功属于高频细节，只在 debug 级别输出
			slog.Debug("通道"+stage+"成功", "channel", ch.ID, "name", ch.Name, "type", ch.Type, "attempt", i+1, "latency_ms", latency)
			if record != nil {
				record(i+1, "success", "", latency)
			}
			return nil
		}
		lastErr = err
		permanent := notifier.IsPermanent(err)
		slog.Warn("通道"+stage+"失败", "channel", ch.ID, "name", ch.Name, "type", ch.Type,
			"attempt", i+1, "of", len(d.RetryDelays), "latency_ms", latency, "permanent", permanent, "error", err)
		if record != nil {
			record(i+1, "failed", err.Error(), latency)
		}
		if permanent {
			return err
		}
	}
	slog.Warn("通道重试次数用尽，最终失败", "channel", ch.ID, "name", ch.Name, "attempts", len(d.RetryDelays))
	return lastErr
}

// sendWithRetry 在单个通道上发送并记录每次尝试，返回是否最终成功。
func (d *DispatchService) sendWithRetry(ctx context.Context, ch *models.Channel, deliveryLogID uint, msg notifier.Message) bool {
	n, err := notifier.Get(ch.Type)
	if err != nil {
		slog.Error("通道类型未注册，无法发送", "log", deliveryLogID, "channel", ch.ID, "name", ch.Name, "type", ch.Type, "error", err)
		d.writeAttempt(deliveryLogID, ch, 1, "failed", err.Error(), 0)
		return false
	}
	plan, err := d.buildPlan(ch, n, msg)
	if err != nil {
		slog.Error("解密通道配置失败，无法发送", "log", deliveryLogID, "channel", ch.ID, "name", ch.Name, "type", ch.Type, "error", err)
		// 落库的提示文案去掉 AppError 的 code/field 前缀，方便用户直接读
		d.writeAttempt(deliveryLogID, ch, 1, "failed", friendlyErr(err), 0)
		return false
	}
	record := func(attempt int, status, errMsg string, latencyMs int) {
		d.writeAttempt(deliveryLogID, ch, attempt, status, errMsg, latencyMs)
	}
	return d.runRetries(ctx, ch, plan, "发送", record) == nil
}

// finalize 更新 DeliveryLog 的终态字段。
func (d *DispatchService) finalize(deliveryLogID uint, status, renderedTitle, renderedContent string) {
	updates := map[string]any{
		"status":  status,
		"title":   renderedTitle,
		"content": renderedContent,
	}
	if err := d.DB.Model(&models.DeliveryLog{}).Where("id = ?", deliveryLogID).Updates(updates).Error; err != nil {
		slog.Error("更新投递日志终态失败", "log", deliveryLogID, "error", err)
	}
}

// loadChannels 取提醒绑定的全部通道；没有绑定时返回空切片。
func (d *DispatchService) loadChannels(reminderID uint) ([]*models.Channel, error) {
	return loadReminderChannels(d.DB, reminderID)
}

// writeAttempt 落一条 DeliveryAttempt。
func (d *DispatchService) writeAttempt(deliveryLogID uint, ch *models.Channel, attempt int, status, errMsg string, latencyMs int) {
	row := &models.DeliveryAttempt{
		DeliveryLogID: deliveryLogID,
		ChannelID:     ch.ID,
		ChannelType:   ch.Type,
		ChannelName:   ch.Name,
		Attempt:       attempt,
		Status:        status,
		Error:         textutil.TruncateRunes(errMsg, 1000),
		LatencyMs:     latencyMs,
	}
	if err := d.DB.Create(row).Error; err != nil {
		slog.Error("写 attempt 失败", "log", deliveryLogID, "channel", ch.ID, "error", err)
	}
}

// DryRun 发送测试消息，不落任何日志；失败时返回 400 + 可读文案而不是 500。
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
				errCh <- fmt.Errorf("%s: %w", ch.Name, err)
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
		return middleware.NewAppError(middleware.CodeValidationFailed, "试发失败: "+strings.Join(errs, "; "))
	}
	return nil
}

// sendDryRun 单通道试发（有重试，不写 attempt）。
func (d *DispatchService) sendDryRun(ctx context.Context, ch *models.Channel, msg notifier.Message) error {
	plan, err := d.prepare(ch, msg)
	if err != nil {
		return err
	}
	if err := d.runRetries(ctx, ch, plan, "试发", nil); err != nil {
		return err
	}
	return nil
}

// --- helpers ---

// friendlyErr 取 AppError 里的 message，去掉 code/field 等技术细节。
func friendlyErr(err error) string {
	var ae *middleware.AppError
	if errors.As(err, &ae) {
		return ae.Message
	}
	return err.Error()
}
