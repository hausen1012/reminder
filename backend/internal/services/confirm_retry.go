// confirm_retry 管理"需要确认"提醒的重发链。
//
// 当提醒触发后未被确认，按配置的间隔与最大次数持续重发；
// 每次重发复用同一个 confirm_url token，任一时刻点击即终止整条链。
package services

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/bedrock/backend/internal/models"
	"github.com/bedrock/backend/internal/notifier"
	"gorm.io/gorm"
)

// ConfirmRetryManager 持有内存中的 chain_id → *time.Timer 映射。
//
// 进程重启后重发链会丢失（设计取舍：确认机制是增强功能而非核心可靠性保障；
// 如需极端可靠应依赖外部消息队列，对本项目的个人使用场景而言不必要）。
type ConfirmRetryManager struct {
	DB          *gorm.DB
	Dispatch    *DispatchService
	ConfirmSvc  *ConfirmService
	Loc         *time.Location

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
		return
	}
	delay := time.Duration(r.ConfirmRetryIntervalSec) * time.Second
	timer := time.AfterFunc(delay, func() {
		m.retry(r.ID, chainID, round+1)
	})
	m.mu.Lock()
	m.timers[chainID] = timer
	m.mu.Unlock()
}

// Cancel 取消指定 chain 的重发（提醒被编辑/关闭时调用）。
func (m *ConfirmRetryManager) Cancel(chainID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if t, ok := m.timers[chainID]; ok {
		t.Stop()
		delete(m.timers, chainID)
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
		// 提醒已被删除
		m.mu.Lock()
		delete(m.timers, chainID)
		m.mu.Unlock()
		return
	}
	// 提醒已禁用或关闭确认
	if !r.Enabled || !r.RequireConfirm {
		m.mu.Lock()
		delete(m.timers, chainID)
		m.mu.Unlock()
		return
	}
	// 已超过最大重试次数（防御性检查）
	if round > r.ConfirmMaxRetries {
		m.mu.Lock()
		delete(m.timers, chainID)
		m.mu.Unlock()
		return
	}

	// 检查该 chain 是否已被确认
	var confirmed int64
	m.DB.Model(&models.DeliveryLog{}).
		Where("confirm_chain_id = ? AND confirmed = ?", chainID, true).
		Count(&confirmed)
	if confirmed > 0 {
		return // 已被确认，终止
	}

	// 查找已有 token
	var tok models.ConfirmToken
	err := m.DB.Joins("JOIN delivery_logs ON delivery_logs.id = confirm_tokens.delivery_log_id").
		Where("delivery_logs.confirm_chain_id = ?", chainID).
		First(&tok).Error
	if err != nil {
		log.Printf("[confirm] 查找 chain=%s token 失败: %v", chainID, err)
		return
	}

	// 校验 token 是否未过期 & 未使用
	if tok.UsedAt != nil {
		return
	}
	if time.Now().After(tok.ExpiresAt) {
		return
	}

	// 构建 vars 并重新渲染
	planned := time.Now()
	if r.NextFireAt != nil {
		planned = *r.NextFireAt
	}
	vars := buildVars(&r, time.Now(), planned, m.Loc)
	vars["confirm_url"] = m.ConfirmSvc.BuildURL(tok.Token)

	rendered := notifier.Message{
		Subject: notifier.Render(r.Title, vars),
		Body:    notifier.Render(r.Content, vars),
		Vars:    vars,
	}

	// 创建新的 delivery_log（同一 chain）
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
		log.Printf("[confirm] 创建重发日志失败 chain=%s: %v", chainID, err)
		return
	}

	// 异步 dispatch
	go func(r models.Reminder, logID uint) {
		ctx, cancel := context.WithTimeout(context.Background(), 70*time.Second)
		defer cancel()
		m.Dispatch.Run(ctx, &r, logID)
	}(r, newLog.ID)

	// 调度下一轮（如果有）
	m.Schedule(&r, chainID, round)
}