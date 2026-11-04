// confirm_retry 管理"需要确认"提醒的重发链。
//
// 当提醒触发后未被确认，按配置的间隔与最大次数持续重发；
// 每次重发复用同一个 confirm_url token，任一时刻点击即终止整条链。
package services

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/reminder/backend/internal/models"
	"github.com/reminder/backend/internal/timezone"
	"gorm.io/gorm"
)

// ConfirmRetryManager 持有内存中的 chain_id → *time.Timer 映射。
//
// 定时器本身无法持久化，但确认链的状态在库里（delivery_logs.confirm_chain_id
// + confirm_tokens）：进程重启后由 RecoverChains 按库里的轮次与上次发送时间
// 重新排下一轮，因此「重启导致催办丢失」不再是取舍——除非进程在重启后
// 再也没起来，那种情况下整个系统都已经停了。
type ConfirmRetryManager struct {
	DB         *gorm.DB
	Dispatch   *DispatchService
	ConfirmSvc *ConfirmService
	TZ         *timezone.Provider

	mu     sync.Mutex
	timers map[string]*time.Timer
}

// NewConfirmRetryManager 构造重发管理器。
func NewConfirmRetryManager(db *gorm.DB, dispatch *DispatchService, confirmSvc *ConfirmService, tz *timezone.Provider) *ConfirmRetryManager {
	if tz == nil {
		tz = timezone.New("")
	}
	return &ConfirmRetryManager{
		DB:         db,
		Dispatch:   dispatch,
		ConfirmSvc: confirmSvc,
		TZ:         tz,
		timers:     make(map[string]*time.Timer),
	}
}

// Schedule 注册一轮重发。
//
// round 是已完成的轮次数（0 = 首次触发已发出，下次是第 1 次重发），
// 因此这里判断的是「已经发满 maxRetries 轮就不再排下一轮」，条件是 round >= max。
// 对应的 retry 入口用的是 round > max：它校验的是「当前这一轮是否超界」，
// 两者相差 1 是语义不同（排下一轮 vs 执行本轮），不是笔误。
func (m *ConfirmRetryManager) Schedule(r *models.Reminder, chainID string, round int) {
	if round >= r.ConfirmMaxRetries {
		slog.Info("跳过调度", "chain", chainID, "round", round, "max_retries", r.ConfirmMaxRetries)
		return
	}
	delay := time.Duration(r.ConfirmRetryIntervalSec) * time.Second
	slog.Info("注册重试定时器", "chain", chainID, "reminder", r.ID, "next_round", round+1, "delay", delay)
	m.scheduleAfter(r, chainID, round+1, delay)
}

// scheduleAfter 在 delay 之后执行第 round 轮重发（round 是即将执行的轮次）。
//
// 启动恢复用它安排「剩下的那部分间隔」，正常路径则由 Schedule 传入完整间隔。
func (m *ConfirmRetryManager) scheduleAfter(r *models.Reminder, chainID string, round int, delay time.Duration) {
	if delay < 0 {
		delay = 0
	}
	timer := time.AfterFunc(delay, func() {
		m.retry(r.ID, chainID, round)
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
		slog.Warn("查询活动确认链失败", "reminder", reminderID, "error", err)
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

// RecoverChains 在进程启动时重建未完成的重发链。
//
// 内存里的 *time.Timer 无法存活到重启之后，但链的状态都在库里：
// 每条链的最新一条日志给出已发到第几轮（retry_round）与上次发送时间（fired_at）。
// 恢复逻辑据此排出下一轮，并扣掉停机期间已经流逝的时间：
//
//   - 停机时间 < 重发间隔：再等「剩余间隔」；
//   - 停机时间 >= 重发间隔：立即补一轮（与 sweeper 补漏的语义一致）。
//
// 三条筛选规则，都是「按链整体判断」而不是「按单条日志判断」：
//
//  1. 每个提醒只恢复最新那条链。确认链的语义是「只追踪最新的一轮」——
//     每轮新触发都会取消旧链，用户点的也只会是最新消息里的链接。
//     若把历史链一并恢复，重启后同一提醒会有多条催办链并发往手机上发。
//  2. 链里只要有一条日志是 confirmed，整条链就作废。不能只看「最新一条的
//     confirmed」：确认与重发之间存在竞态窗口，确认之后可能又插进一条
//     confirmed=false 的重发日志，那不代表用户没处理过。
//  3. 令牌已过期（或根本没有令牌）的链不恢复：链接点不开，催也没用。
//
// 就绪判断（是否已用尽、提醒是否还生效）仍然交给 retry：那里已有完整且被
// 测试覆盖的终止条件，这里再判一遍只会多出一份可能漂移的规则。
//
// 返回恢复的链数量。
func (m *ConfirmRetryManager) RecoverChains() (int, error) {
	type chainRow struct {
		ChainID    string `gorm:"column:chain_id"`
		ReminderID uint   `gorm:"column:reminder_id"`
		Round      int    `gorm:"column:round"`
		LastFired  int64  `gorm:"column:last_fired"`
	}

	// 一条 SQL 取回所有候选。每个提醒只在 fired_at/id 最新的一行上留 rn=1，
	// 由此拿到「该提醒最新那条链的最新一轮」。
	//
	// 令牌挂在链的首条日志上（CreateToken 传的是首次触发的 deliveryLogID），
	// 而重发轮的日志没有对应的 token 行，因此令牌要按 chain_id 聚合，
	// 不能直接 LEFT JOIN 到 dl.id 上——否则第 2 轮以后的链会被误判成「没有令牌」。
	//
	// NOT EXISTS 让「已确认」成为链级判断；时间条件放在 SQL 里，
	// 停机很久的旧链不会被带回 Go 再筛。
	var rows []chainRow
	err := m.DB.Raw(`
		SELECT t.chain_id, t.reminder_id, t.round, t.last_fired
		FROM (
			SELECT dl.confirm_chain_id AS chain_id,
			       dl.reminder_id      AS reminder_id,
			       dl.retry_round      AS round,
			       dl.fired_at         AS last_fired,
			       ROW_NUMBER() OVER (
			           PARTITION BY dl.reminder_id
			           ORDER BY dl.fired_at DESC, dl.id DESC
			       ) AS rn
			FROM delivery_logs dl
			WHERE dl.confirm_chain_id IS NOT NULL
			  AND dl.confirm_chain_id != ''
		) t
		JOIN (
			SELECT dl2.confirm_chain_id AS chain_id, MAX(ct.expires_at) AS token_expires
			FROM confirm_tokens ct
			JOIN delivery_logs dl2 ON dl2.id = ct.delivery_log_id
			WHERE dl2.confirm_chain_id IS NOT NULL AND dl2.confirm_chain_id != ''
			GROUP BY dl2.confirm_chain_id
		) tk ON tk.chain_id = t.chain_id
		WHERE t.rn = 1
		  AND tk.token_expires > ?
		  AND NOT EXISTS (
		      SELECT 1 FROM delivery_logs d2
		      WHERE d2.confirm_chain_id = t.chain_id AND d2.confirmed = ?
		  )
	`, time.Now().UnixMilli(), true).Scan(&rows).Error
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}

	// 一次取回涉及的提醒，避免每个链一次查询（N+1）。
	// 启动阶段链可能不少，而这个查询本身与链数量无关。
	ids := make([]uint, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ReminderID)
	}
	var reminders []models.Reminder
	if err := m.DB.Where("id IN ?", ids).Find(&reminders).Error; err != nil {
		return 0, err
	}
	byID := make(map[uint]*models.Reminder, len(reminders))
	for i := range reminders {
		byID[reminders[i].ID] = &reminders[i]
	}

	recovered := 0
	now := time.Now()
	for _, row := range rows {
		r := byID[row.ReminderID]
		if r == nil {
			// 提醒已被删除：整条链作废，不需要记错误
			slog.Debug("恢复确认链时提醒不存在，跳过", "chain", row.ChainID, "reminder", row.ReminderID)
			continue
		}
		if !r.Enabled || !r.RequireConfirm {
			continue
		}
		if row.Round >= r.ConfirmMaxRetries {
			continue
		}
		if m.hasTimer(row.ChainID) {
			continue
		}

		next := row.Round + 1
		interval := time.Duration(r.ConfirmRetryIntervalSec) * time.Second
		elapsed := now.Sub(time.UnixMilli(row.LastFired))
		delay := interval - elapsed
		if delay < 0 {
			delay = 0
		}
		m.scheduleAfter(r, row.ChainID, next, delay)
		recovered++
		slog.Info("恢复确认重发链", "chain", row.ChainID, "reminder", r.ID,
			"next_round", next, "delay", delay)
	}
	if recovered > 0 {
		slog.Info("已恢复未完成的确认重发链", "count", recovered)
	}
	return recovered, nil
}

// hasTimer 判断某条链当前是否已有定时器。
func (m *ConfirmRetryManager) hasTimer(chainID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.timers[chainID]
	return ok
}

// retry 是 AfterFunc 回调：检查条件、创建新日志、重新分发。
func (m *ConfirmRetryManager) retry(reminderID uint, chainID string, round int) {
	var r models.Reminder
	if err := m.DB.First(&r, reminderID).Error; err != nil {
		// 记录不存在是正常终止路径；其他 DB 错误同样会落到这里，一并带上原因
		slog.Info("停止确认链", "chain", chainID, "reason", "提醒不存在", "reminder", reminderID, "error", err)
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
	if err := m.DB.Model(&models.DeliveryLog{}).
		Where("confirm_chain_id = ? AND confirmed = ?", chainID, true).
		Count(&confirmed).Error; err != nil {
		// 查不到确认状态就不能冒险继续重发：宁可这一轮跳过，也不要给已确认的用户再发一遍
		slog.Error("查询确认状态失败，本轮重发跳过", "chain", chainID, "round", round, "error", err)
		m.stopChain(chainID)
		return
	}
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
		slog.Warn("停止确认链", "chain", chainID, "reason", "查找 token 失败", "round", round, "error", err)
		m.stopChain(chainID)
		return
	}
	if tok.UsedAt != nil {
		slog.Info("停止确认链", "chain", chainID, "reason", "确认链接已使用")
		m.stopChain(chainID)
		return
	}
	if time.Now().After(tok.ExpiresAt.Time) {
		slog.Info("停止确认链", "chain", chainID, "reason", "确认链接已过期")
		m.stopChain(chainID)
		return
	}

	planned := time.Now()
	if r.NextFireAt != nil {
		planned = r.NextFireAt.Std()
	}
	vars := buildVars(&r, time.Now(), planned, m.TZ.Get())
	vars["confirm_url"] = m.ConfirmSvc.BuildURL(tok.Token)

	rendered := buildRenderedMessage(r.Title, r.Content, r.ContentFormat, vars)

	newLog := &models.DeliveryLog{
		ReminderID:     r.ID,
		FiredAt:        models.TimeNow(),
		Title:          rendered.Subject,
		Content:        rendered.Body,
		Status:         "pending",
		Source:         r.Source,
		RetryRound:     round,
		ConfirmChainID: &chainID,
	}
	if err := m.DB.Create(newLog).Error; err != nil {
		slog.Error("创建重发日志失败，确认链终止", "chain", chainID, "round", round, "reminder", r.ID, "error", err)
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
