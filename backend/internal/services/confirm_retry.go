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
		log.Printf("[confirm] 跳过调度 chain=%s：round=%d 已达到上限 %d", chainID, round, r.ConfirmMaxRetries)
		return
	}
	delay := time.Duration(r.ConfirmRetryIntervalSec) * time.Second
	log.Printf("[confirm] 注册重试定时器 chain=%s reminder=%d next_round=%d delay=%s", chainID, r.ID, round+1, delay)
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
		log.Printf("[confirm] 查询 reminder=%d 活动确认链失败: %v", reminderID, err)
		return
	}
	for _, chainID := range chainIDs {
		if m.stopChain(chainID) {
			log.Printf("[confirm] 取消 reminder=%d 的确认链 chain=%s", reminderID, chainID)
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
		log.Printf("[confirm] 停止确认链 chain=%s：提醒不存在", chainID)
		m.stopChain(chainID)
		return
	}
	if !r.Enabled {
		log.Printf("[confirm] 停止确认链 chain=%s：提醒已禁用", chainID)
		m.stopChain(chainID)
		return
	}
	if !r.RequireConfirm {
		log.Printf("[confirm] 停止确认链 chain=%s：提醒已关闭确认", chainID)
		m.stopChain(chainID)
		return
	}
	if round > r.ConfirmMaxRetries {
		log.Printf("[confirm] 停止确认链 chain=%s：已达到最大重试次数 %d", chainID, r.ConfirmMaxRetries)
		m.stopChain(chainID)
		return
	}

	var confirmed int64
	m.DB.Model(&models.DeliveryLog{}).
		Where("confirm_chain_id = ? AND confirmed = ?", chainID, true).
		Count(&confirmed)
	if confirmed > 0 {
		log.Printf("[confirm] 停止确认链 chain=%s：用户已确认", chainID)
		m.stopChain(chainID)
		return
	}

	var tok models.ConfirmToken
	err := m.DB.Joins("JOIN delivery_logs ON delivery_logs.id = confirm_tokens.delivery_log_id").
		Where("delivery_logs.confirm_chain_id = ?", chainID).
		First(&tok).Error
	if err != nil {
		log.Printf("[confirm] 停止确认链 chain=%s：查找 token 失败: %v", chainID, err)
		m.stopChain(chainID)
		return
	}
	if tok.UsedAt != nil {
		log.Printf("[confirm] 停止确认链 chain=%s：确认链接已使用", chainID)
		m.stopChain(chainID)
		return
	}
	if time.Now().After(tok.ExpiresAt) {
		log.Printf("[confirm] 停止确认链 chain=%s：确认链接已过期", chainID)
		m.stopChain(chainID)
		return
	}

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
		m.stopChain(chainID)
		return
	}
	log.Printf("[confirm] 触发第 %d 次重发 chain=%s reminder=%d log=%d", round, chainID, r.ID, newLog.ID)

	go func(r models.Reminder, logID uint) {
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
