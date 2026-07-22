// confirm_retry 管理"需要确认"提醒的重发链。
//
// 当提醒触发后未被确认，按配置的间隔与最大次数持续重发；
// 每次重发复用同一个 confirm_url token，任一时刻点击即终止整条链。
package services

import (
	"context"
	"sync"
	"time"
	"log/slog"

	"github.com/reminder/backend/internal/models"
	"gorm.io/gorm"
)

// ConfirmRetryManager 持有内存中的 chain_id → *time.Timer 映射。
//
// 进程重启后重发链会丢失（设计取舍：确认机制是增强功能而非核心可靠性保障；
// 如需极端可靠应依赖外部消息队列，对本项目的个人使用场景而言不必要）。
type ConfirmRetryManager struct {
	DB         *gorm.DB
	Dispatch   *DispatchService
	ConfirmSvc *ConfirmService
	Loc        *time.Location

	mu     sync.Mutex
	timers map[string]*time.Timer
}

// NewConfirmRetryManager 构造重发管理器。
func NewConfirmRetryManager(db *gorm.DB, dispatch *DispatchService, confirmSvc *ConfirmService, loc *time.Location) *ConfirmRetryManager {
	return &ConfirmRetryManager{
		DB:         db,
		Dispatch:   dispatch,
		ConfirmSvc: confirmSvc,
		Loc:        loc,
		timers:     make(map[string]*time.Timer),
	}
}

// Schedule 注册一轮重发。
//
// round 是已完成的轮次数（0 = 首次触发已发出，下次是第 1 次重发）。
func (m *ConfirmRetryManager) Schedule(r *models.Reminder, chainID string, round int) {
	if round >= r.ConfirmMaxRetries {
		slog.Info("跳过调度", "chain", chainID, "round", round, "max_retries", r.ConfirmMaxRetries)
		return
	}
	delay := time.Duration(r.ConfirmRetryIntervalSec) * time.Second
	slog.Info("注册重试定时器", "chain", chainID, "reminder", r.ID, "next_round", round+1, "delay", delay)
	timer := time.AfterFunc(delay, func() {
		m.retry(r.ID, chainID, round+1)
	})
	m.mu.Lock()
	m.timers[chainID] = timer
	m.mu.Unlock()
}

// Cancel 取消指定 chain 的重发（提醒被编辑/关闭时调用）。
func (m *ConfirmRetryManager) Cancel(chainID string) {
	m.stopChain(chainID)
}

// CancelByReminderID 取消某条提醒当前所有活动确认链（仅在内存中有定时器时记录日志）。
func (m *ConfirmRetryManager) CancelByReminderID(reminderID uint) {
	if reminderID == 0 {
		return
	}
	var chainIDs []string
	if err := m.DB.Model(&models.DeliveryLog{}).
		Distinct("confirm_chain_id").
		Where("reminder_id = ? AND confirm_chain_id IS NOT NULL AND confirm_chain_id != '' AND confirmed = ?", reminderID, false).
		Pluck("confirm_chain_id", &chainIDs).Error; err != nil {
		slog.Info("查询活动确认链失败", "reminder", reminderID, "error", err)
		return
	}
	for _, chainID := range chainIDs {
		if m.stopChain(chainID) {
			slog.Info("取消确认链", "reminder", reminderID, "chain", chainID)
		}
	}
}

// StopAll 取消所有重发（进程退出前调用）。
func (m *ConfirmRetryManager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, t := range m.timers {
		t.Stop()
		delete(m.timers, id)
	}
}

// retry 是 AfterFunc 回调：检查条件、创建新日志、重新分发。
func (m *ConfirmRetryManager) retry(reminderID uint, chainID string, round int) {
	var r models.Reminder
	if err := m.DB.First(&r, reminderID).Error; err != nil {
		slog.Info("停止确认链", "chain", chainID, "reason", "提醒不存在")
		m.stopChain(chainID)
		return
	}
	if !r.Enabled {
		slog.Info("停止确认链", "chain", chainID, "reason", "提醒已禁用")
		m.stopChain(chainID)
		return
	}
	if !r.RequireConfirm {
		slog.Info("停止确认链", "chain", chainID, "reason", "提醒已关闭确认")
		m.stopChain(chainID)
		return
	}
	if round > r.ConfirmMaxRetries {
		slog.Info("停止确认链", "chain", chainID, "reason", "已达到最大重试次数", "max_retries", r.ConfirmMaxRetries)
		m.stopChain(chainID)
		return
	}

	var confirmed int64
	m.DB.Model(&models.DeliveryLog{}).
		Where("confirm_chain_id = ? AND confirmed = ?", chainID, true).
		Count(&confirmed)
	if confirmed > 0 {
		slog.Info("停止确认链", "chain", chainID, "reason", "用户已确认")
		m.stopChain(chainID)
		return
	}

	var tok models.ConfirmToken
	err := m.DB.Joins("JOIN delivery_logs ON delivery_logs.id = confirm_tokens.delivery_log_id").
		Where("delivery_logs.confirm_chain_id = ?", chainID).
		First(&tok).Error
	if err != nil {
		slog.Info("停止确认链", "chain", chainID, "reason", "查找 token 失败", "error", err)
		m.stopChain(chainID)
		return
	}
	if tok.UsedAt != nil {
		slog.Info("停止确认链", "chain", chainID, "reason", "确认链接已使用")
		m.stopChain(chainID)
		return
	}
	if time.Now().After(tok.ExpiresAt) {
		slog.Info("停止确认链", "chain", chainID, "reason", "确认链接已过期")
		m.stopChain(chainID)
		return
	}

	planned := time.Now()
	if r.NextFireAt != nil {
		planned = *r.NextFireAt
	}
	vars := buildVars(&r, time.Now(), planned, m.Loc)
	vars["confirm_url"] = m.ConfirmSvc.BuildURL(tok.Token)

	rendered := buildRenderedMessage(r.Title, r.Content, r.ContentFormat, vars)

	newLog := &models.DeliveryLog{
		ReminderID:     r.ID,
		FiredAt:        time.Now(),
		Title:          rendered.Subject,
		Content:        rendered.Body,
		Status:         "pending",
		Source:         r.Source,
		RetryRound:     round,
		ConfirmChainID: &chainID,
	}
	if err := m.DB.Create(newLog).Error; err != nil {
		slog.Info("创建重发日志失败", "chain", chainID, "error", err)
		m.stopChain(chainID)
		return
	}
	slog.Info("触发重发", "round", round, "chain", chainID, "reminder", r.ID, "log", newLog.ID)

	go func(r models.Reminder, logID uint) {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("confirm retry goroutine panic", "recover", r, "log", logID)
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 70*time.Second)
		defer cancel()
		m.Dispatch.Run(ctx, &r, logID)
	}(r, newLog.ID)

	m.Schedule(&r, chainID, round)
}

// stopChain 停止内存中指定 chain 的定时器。返回 true 表示确实取消了活动定时器。
func (m *ConfirmRetryManager) stopChain(chainID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if t, ok := m.timers[chainID]; ok {
		t.Stop()
		delete(m.timers, chainID)
		return true
	}
	return false
}
