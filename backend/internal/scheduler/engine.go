// Engine 是调度器的核心：把 Reminder 按其 (Calendar, ScheduleType) 注册到
// 合适的触发器上（cron.AddFunc 或 time.AfterFunc），到点触发 fire 闭包。
//
// fire 闭包负责：
//  1. 重读最新 Reminder（防止过时缓存）
//  2. 在短事务内乐观锁更新 next_fire_at + 插入 pending delivery_log
//  3. 异步把 dispatch 任务交给 DispatchService
//  4. 若非 cron 触发器，重新算下一次并注册新的 AfterFunc
package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/reminder/backend/internal/models"
	"github.com/reminder/backend/internal/timezone"
	"github.com/robfig/cron/v3"
	"gorm.io/gorm"
)

// Dispatcher 是 Engine 调用 dispatch 阶段的接口，避免循环依赖。
//
// 实现：services.DispatchService。
type Dispatcher interface {
	Run(ctx context.Context, reminder *models.Reminder, deliveryLogID uint)
}

// Engine 持有 cron 实例与已注册条目的内存映射。
type Engine struct {
	db   *gorm.DB
	cron *cron.Cron
	disp Dispatcher
	tz   *timezone.Provider

	mu  sync.Mutex
	reg map[uint]*registered

	stopped    bool
	started    bool
	lastFireMu sync.Mutex
	lastFireAt time.Time
}

type registeredKind string

const (
	kindCron      registeredKind = "cron"
	kindAfterFunc registeredKind = "afterfunc"
)

// registered 记录一条提醒在 Engine 内部的注册形态。
type registered struct {
	kind    registeredKind
	entryID cron.EntryID // kind=cron 时有效
	timer   *time.Timer  // kind=afterfunc 时有效
}

// NewEngine 构造 Engine；时区用于 cron 解析与 AfterFunc 计算。
//
// 传入 Provider 而不是固定的 *time.Location：站点时区可以在设置页热更新，
// 每次计算都会读取当时的时区。
func NewEngine(db *gorm.DB, disp Dispatcher, tz *timezone.Provider) *Engine {
	if tz == nil {
		tz = timezone.New("")
	}
	c := cron.New(cron.WithLocation(tz.Get()), cron.WithSeconds())
	return &Engine{
		db:   db,
		cron: c,
		disp: disp,
		tz:   tz,
		reg:  make(map[uint]*registered),
	}
}

// ReloadTimezone 在站点时区变更后重建 cron 实例并重新注册全部提醒。
//
// robfig/cron 的时区在构造时固定，改时区只能重建实例；
// 已注册的 AfterFunc 也要按新时区重算下次触发时间。
func (e *Engine) ReloadTimezone() {
	e.mu.Lock()
	wasStarted := e.started && !e.stopped
	old := e.cron
	// 先摘掉所有注册项，避免旧的 AfterFunc 在新时区下继续触发
	for _, r := range e.reg {
		if r.timer != nil {
			r.timer.Stop()
		}
	}
	e.reg = make(map[uint]*registered)
	e.cron = cron.New(cron.WithLocation(e.tz.Get()), cron.WithSeconds())
	e.mu.Unlock()

	// cron.Stop 会等待正在执行的 job 结束，其中可能回调 fire 去拿 e.mu，
	// 因此必须在释放锁之后调用。
	if wasStarted && old != nil {
		if ctx := old.Stop(); ctx != nil {
			<-ctx.Done()
		}
	}

	if wasStarted {
		e.cron.Start()
	}
	if err := e.LoadAndRegisterAll(); err != nil {
		slog.Warn("时区变更后重新注册提醒失败", "error", err)
	}
}

// Start 启动内部 cron 循环。Sweeper 由外层独立启动。
func (e *Engine) Start() {
	e.mu.Lock()
	e.started = true
	e.mu.Unlock()
	e.cron.Start()
}

// Stop 停止内部 cron 循环并取消所有 AfterFunc。
func (e *Engine) Stop() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.stopped {
		return
	}
	ctx := e.cron.Stop()
	<-ctx.Done()
	for _, r := range e.reg {
		if r.timer != nil {
			r.timer.Stop()
		}
	}
	e.reg = make(map[uint]*registered)
	e.stopped = true
	e.started = false
}

// Add 注册一条提醒到调度器。
//
// 若 reminder 已被注册，先 Remove 再注册（等价于 Update）。
func (e *Engine) Add(r *models.Reminder) error {
	if r == nil {
		return errors.New("reminder 不能为空")
	}
	if !r.Enabled {
		// 禁用的提醒不进入调度
		e.Remove(r.ID)
		return nil
	}
	e.Remove(r.ID)

	spec, err := ParseSpec(r.ScheduleSpec)
	if err != nil {
		return err
	}

	switch {
	case r.Calendar == "solar" && r.ScheduleType == "cron":
		return e.addCron(r, spec)
	case r.Calendar == "solar" && (r.ScheduleType == "once" || r.ScheduleType == "interval"):
		return e.addAfterFunc(r, spec)
	case r.Calendar == "lunar" && (r.ScheduleType == "once" || r.ScheduleType == "interval"):
		return e.addAfterFunc(r, spec)
	}
	return nil
}

// Update 等价于 Add（Add 内部会先 Remove）。
func (e *Engine) Update(r *models.Reminder) error {
	return e.Add(r)
}

// Remove 把一条提醒从调度器移除。
func (e *Engine) Remove(id uint) {
	e.mu.Lock()
	defer e.mu.Unlock()
	r, ok := e.reg[id]
	if !ok {
		return
	}
	switch r.kind {
	case kindCron:
		e.cron.Remove(r.entryID)
	case kindAfterFunc:
		if r.timer != nil {
			r.timer.Stop()
		}
	}
	delete(e.reg, id)
}

// addCron 把 solar/cron 注册到 cron.Cron 实例。
//
// robfig/cron 在 WithSeconds() 下需要 6 字段；外部 spec 给的是标准 5 字段，
// 这里手动补 "0 " 让两者兼容（用户配置含义不变）。
func (e *Engine) addCron(r *models.Reminder, spec *ScheduleSpec) error {
	if spec.Expr == "" {
		return errors.New("cron 表达式为空")
	}
	id := r.ID
	// 持锁操作 cron 实例：ReloadTimezone 会整体替换 e.cron
	e.mu.Lock()
	defer e.mu.Unlock()
	entryID, err := e.cron.AddFunc("0 "+spec.Expr, func() {
		e.fire(id)
	})
	if err != nil {
		return err
	}
	e.reg[id] = &registered{kind: kindCron, entryID: entryID}
	return nil
}

// addAfterFunc 为 solar/once 与 solar/interval 注册一次性 AfterFunc。
//
// 若计算出的下次触发为 nil（spec 已过期）或在过去太久，直接不注册。
func (e *Engine) addAfterFunc(r *models.Reminder, spec *ScheduleSpec) error {
	if r == nil {
		return errors.New("reminder 不能为空")
	}
	if !r.Enabled {
		e.Remove(r.ID)
		return nil
	}
	now := time.Now()
	next, err := Compute(r.Calendar, r.ScheduleType, spec, now, e.tz.Get())
	if err != nil {
		return err
	}
	if next == nil {
		e.Remove(r.ID)
		return nil
	}
	d := time.Until(*next)
	if d < 0 {
		// 过去时间：留给 sweeper 处理；为安全起见也不阻塞调度。
		d = 0
	}
	id := r.ID
	timer := time.AfterFunc(d, func() {
		e.fire(id)
	})
	e.mu.Lock()
	e.reg[id] = &registered{kind: kindAfterFunc, timer: timer}
	e.mu.Unlock()
	return nil
}

// RegisteredCount 返回当前注册的提醒数量。
func (e *Engine) RegisteredCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.reg)
}

// RegisteredEntry 对外暴露注册条目视图。
type RegisteredEntry struct {
	ID           uint       `json:"id"`
	Kind         string     `json:"kind"`
	NextFireAt   *time.Time `json:"next_fire_at"`
	Title        string     `json:"title"`
	LastFiredAt  *time.Time `json:"last_fired_at"`
	ScheduleType string     `json:"schedule_type"`
}

// ListRegistered 返回当前 Engine 中所有已注册条目的快照。
func (e *Engine) ListRegistered() []RegisteredEntry {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]RegisteredEntry, 0, len(e.reg))
	for id, r := range e.reg {
		ent := RegisteredEntry{
			ID:   id,
			Kind: string(r.kind),
		}
		var reminder models.Reminder
		if err := e.db.Select("next_fire_at, title, last_fired_at, schedule_type").First(&reminder, id).Error; err == nil {
			ent.NextFireAt = reminder.NextFireAt
			ent.Title = reminder.Title
			ent.LastFiredAt = reminder.LastFiredAt
			ent.ScheduleType = reminder.ScheduleType
		}
		out = append(out, ent)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// ListRegisteredSorted 返回按 next_fire_at 升序排列的分页注册条目。
// nil next_fire_at 排在最后。offset/limit 为 0 时返回全部。
func (e *Engine) ListRegisteredSorted(offset, limit int) (entries []RegisteredEntry, total int) {
	all := e.ListRegistered()
	total = len(all)

	sort.Slice(all, func(i, j int) bool {
		if all[i].NextFireAt == nil && all[j].NextFireAt == nil {
			return all[i].ID < all[j].ID
		}
		if all[i].NextFireAt == nil {
			return false
		}
		if all[j].NextFireAt == nil {
			return true
		}
		if all[i].NextFireAt.Equal(*all[j].NextFireAt) {
			return all[i].ID < all[j].ID
		}
		return all[i].NextFireAt.Before(*all[j].NextFireAt)
	})

	if offset >= total {
		return nil, total
	}
	end := offset + limit
	if limit <= 0 || end > total {
		end = total
	}
	return all[offset:end], total
}

// IsRunning 返回调度器是否已启动且未停止。
func (e *Engine) IsRunning() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.started && !e.stopped
}

// LastFireTime 返回最近一次 fire 的时间。
func (e *Engine) LastFireTime() time.Time {
	e.lastFireMu.Lock()
	defer e.lastFireMu.Unlock()
	return e.lastFireAt
}

// fire 是所有触发器的统一入口。
//
// 步骤：
//  1. 重读最新 reminder；不存在/已禁用直接 return
//  2. 短事务：乐观锁更新 next_fire_at + 插入 delivery_log(pending)
//  3. 异步交给 dispatcher
//  4. 若非 cron 触发器，注册下一次 AfterFunc
func (e *Engine) fire(id uint) {
	e.lastFireMu.Lock()
	e.lastFireAt = time.Now()
	e.lastFireMu.Unlock()

	var r models.Reminder
	if err := e.db.First(&r, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return
		}
		slog.Info("读取 reminder 失败", "reminder", id, "error", err)
		return
	}
	if !r.Enabled {
		return
	}

	spec, err := ParseSpec(r.ScheduleSpec)
	if err != nil {
		slog.Info("解析 reminder spec 失败", "reminder", id, "error", err)
		return
	}

	now := time.Now()
	// 算出新的 next_fire_at（cron 也要算，用于持久化展示）
	newNext, err := Compute(r.Calendar, r.ScheduleType, spec, now, e.tz.Get())
	if err != nil {
		slog.Info("计算下次触发失败", "reminder", id, "error", err)
	}
	// 统一以 UTC 落库：SQLite 按文本比较时间，偏移不同的行会被排错位置
	nowUTC := timezone.UTC(now)
	newNext = timezone.UTCPtr(newNext)

	var logID uint
	err = e.db.Transaction(func(tx *gorm.DB) error {
		// 乐观锁：用旧 next_fire_at 防止重复触发。
		// 用 julianday() 比较而不是文本等值：升级前写入的行是 +08:00 偏移，
		// 文本与 UTC 边界永远不相等，会让老提醒既不触发也不报错。
		oldNext := r.NextFireAt
		q := tx.Model(&models.Reminder{}).Where("id = ? AND enabled = ?", id, true)
		if oldNext != nil {
			q = q.Where("julianday(next_fire_at) = julianday(?)", *oldNext)
		} else {
			q = q.Where("next_fire_at IS NULL")
		}
		updates := map[string]any{
			"last_fired_at": nowUTC,
			"next_fire_at":  newNext,
			"fire_count":    gorm.Expr("fire_count + 1"),
		}
		res := q.Updates(updates)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			// 别的路径（sweeper 或并发触发）已经处理过这条，跳过
			return errSkipped
		}
		// 插入 pending 日志
		titleSnapshot := r.Title
		contentSnapshot := r.Content
		dlog := &models.DeliveryLog{
			ReminderID: r.ID,
			FiredAt:    nowUTC,
			Title:      titleSnapshot,
			Content:    contentSnapshot,
			Status:     "pending",
			Source:     r.Source,
			RetryRound: 0,
		}
		if err := tx.Create(dlog).Error; err != nil {
			return err
		}
		logID = dlog.ID
		return nil
	})
	if err != nil {
		if errors.Is(err, errSkipped) {
			return
		}
		slog.Info("fire 事务失败", "reminder", id, "error", err)
		return
	}

	// 异步执行 dispatch，避免阻塞调度循环
	if e.disp != nil {
		go func(r models.Reminder, logID uint) {
			ctx, cancel := context.WithTimeout(context.Background(), 70*time.Second)
			defer cancel()
			e.disp.Run(ctx, &r, logID)
		}(r, logID)
	}

	// 非 cron 触发器需要重新注册下一次 AfterFunc
	if r.ScheduleType != "cron" {
		// 重新读一次，拿到最新的 NextFireAt（事务已写入）
		var rr models.Reminder
		if err := e.db.First(&rr, id).Error; err == nil {
			if err := e.addAfterFunc(&rr, spec); err != nil {
				slog.Info("reschedule reminder 失败", "reminder", id, "error", err)
			}
		}
	}
}

// errSkipped 用于事务内传达"此次 fire 被乐观锁拦截"，外层静默忽略。
var errSkipped = errors.New("skipped by optimistic lock")

// LoadAndRegisterAll 启动时一次性把所有 enabled 提醒注册进 Engine。
func (e *Engine) LoadAndRegisterAll() error {
	var items []models.Reminder
	if err := e.db.Where("enabled = ?", true).Find(&items).Error; err != nil {
		return err
	}
	for i := range items {
		if err := e.Add(&items[i]); err != nil {
			slog.Info("注册 reminder 失败", "reminder", items[i].ID, "error", err)
		}
	}
	return nil
}

// AddPurgeCron 注册一个每天凌晨 3 点的定时清理任务。
func (e *Engine) AddPurgeCron(logSrv interface {
	Purge(olderThan time.Duration, all bool) (int64, error)
}, purgeAfter time.Duration) (cron.EntryID, error) {
	return e.cron.AddFunc("0 0 3 * * *", func() {
		n, err := logSrv.Purge(purgeAfter, false)
		if err != nil {
			slog.Info("自动清理日志失败", "error", err)
		} else if n > 0 {
			slog.Info("自动清理日志", "count", n)
		}
	})
}

// debugString 用于测试/排查内部状态。
func (e *Engine) debugString() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	b, _ := json.Marshal(struct {
		Count int `json:"count"`
	}{Count: len(e.reg)})
	return string(b)
}
