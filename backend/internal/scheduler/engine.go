// Engine 负责提醒到期触发、乐观锁推进下次时间并调度投递。
package scheduler

import (
	"context"
	"errors"
	"log/slog"
	"runtime/debug"
	"sort"
	"sync"
	"time"

	"github.com/reminder/backend/internal/models"
	"github.com/reminder/backend/internal/timezone"
	"github.com/robfig/cron/v3"
	"gorm.io/gorm"
)

// Dispatcher 是 Engine 触发后交给下游的接口，避免循环依赖（实现：services.DispatchService）。
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

	// reloadMu 串行化 ReloadTimezone，避免并发改时区时交错重建 cron 实例。
	reloadMu sync.Mutex

	// purgeJobs 是维护型定时任务，ReloadTimezone 替换 cron 实例时需一并恢复。
	purgeJobs []purgeJob

	stopped    bool
	started    bool
	lastFireMu sync.Mutex
	lastFireAt time.Time
}

// purgeJob 是一条维护型 cron 任务的重建信息。
type purgeJob struct {
	expr string
	job  func()
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

// NewEngine 构造 Engine；传入 Provider 而非固定时区，以支持设置页热更新。
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

// ReloadTimezone 在站点时区变更后重建 cron 实例、重算下次触发时间并重新注册全部提醒。
func (e *Engine) ReloadTimezone() {
	e.reloadMu.Lock()
	defer e.reloadMu.Unlock()

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
	// 维护型任务（如每日日志清理）也要在新实例上恢复
	for _, job := range e.purgeJobs {
		if _, err := e.cron.AddFunc(job.expr, job.job); err != nil {
			slog.Warn("时区变更后恢复维护任务失败", "expr", job.expr, "error", err)
		}
	}
	e.mu.Unlock()

	// cron.Stop 会等待正在执行的 job 结束，其中可能回调 fire 去拿 e.mu，须在释放锁后调用
	if wasStarted && old != nil {
		if ctx := old.Stop(); ctx != nil {
			<-ctx.Done()
		}
	}

	if wasStarted {
		e.cron.Start()
	}
	// 先回写库里的下次触发时间，再注册（addAfterFunc 会重算并覆盖内存定时器）
	if n, err := e.RecomputeNextFire(); err != nil {
		slog.Warn("时区变更后重算下次触发时间失败", "error", err)
	} else {
		slog.Info("时区变更后已重算下次触发时间", "count", n, "timezone", e.tz.Name())
	}
	if err := e.LoadAndRegisterAll(); err != nil {
		slog.Warn("时区变更后重新注册提醒失败", "error", err)
	}
}

// RecomputeNextFire 按当前站点时区以当前时间为基准重算所有启用提醒的 next_fire_at 并回写。
func (e *Engine) RecomputeNextFire() (int, error) {
	var items []models.Reminder
	if err := e.db.Where("enabled = ?", true).Find(&items).Error; err != nil {
		return 0, err
	}
	now := time.Now()
	loc := e.tz.Get()
	updated := 0
	for i := range items {
		r := &items[i]
		spec, err := ParseSpec(r.ScheduleSpec)
		if err != nil {
			slog.Warn("重算时解析 schedule_spec 失败，跳过该提醒", "reminder", r.ID, "title", r.Title, "error", err)
			continue
		}
		next, err := Compute(r.Calendar, r.ScheduleType, spec, now, loc)
		if err != nil {
			slog.Warn("重算下次触发失败，跳过该提醒", "reminder", r.ID, "title", r.Title, "error", err)
			continue
		}
		// 显式用 any(nil)：直接传 *models.Time(nil) 到 GORM 的 Update 里
		// 类型断言路径不稳，容易把 NULL 写成零值时间。
		var val any
		if next != nil {
			val = models.TimeOf(*next)
		}
		if err := e.db.Model(&models.Reminder{}).Where("id = ?", r.ID).
			Update("next_fire_at", val).Error; err != nil {
			slog.Warn("回写下次触发时间失败", "reminder", r.ID, "error", err)
			continue
		}
		updated++
	}
	return updated, nil
}

// ExtraEntryCount 返回 cron 实例里除提醒之外的任务数量，用于断言时区重建后维护任务未丢失。
func (e *Engine) ExtraEntryCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	reminderEntries := make(map[cron.EntryID]struct{}, len(e.reg))
	for _, r := range e.reg {
		if r.kind == kindCron {
			reminderEntries[r.entryID] = struct{}{}
		}
	}
	extra := 0
	for _, ent := range e.cron.Entries() {
		if _, ok := reminderEntries[ent.ID]; !ok {
			extra++
		}
	}
	return extra
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

// addCron 把 solar/cron 注册到 cron 实例；配置给的是 5 字段，这里补上秒位。
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

// addAfterFunc 为 once / interval 注册一次性 AfterFunc；下次触发为 nil（已过期）时不注册。
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

// ChannelInfo 表示注册条目关联的通道简要信息。
type ChannelInfo struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

// RegisteredEntry 对外暴露注册条目视图。
type RegisteredEntry struct {
	ID           uint          `json:"id"`
	Kind         string        `json:"kind"`
	NextFireAt   *models.Time  `json:"next_fire_at"`
	Title        string        `json:"title"`
	LastFiredAt  *models.Time  `json:"last_fired_at"`
	ScheduleType string        `json:"schedule_type"`
	Channels     []ChannelInfo `json:"channels"`
}

// ListRegistered 返回当前 Engine 中所有已注册条目的快照。
func (e *Engine) ListRegistered() []RegisteredEntry {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]RegisteredEntry, 0, len(e.reg))
	if len(e.reg) == 0 {
		return out
	}

	ids := make([]uint, 0, len(e.reg))
	for id := range e.reg {
		ids = append(ids, id)
	}

	var reminders []models.Reminder
	if err := e.db.Select("id, next_fire_at, title, last_fired_at, schedule_type").
		Where("id IN ?", ids).
		Find(&reminders).Error; err != nil {
		slog.Error("ListRegistered 查询提醒失败", "error", err)
	}

	reminderMap := make(map[uint]models.Reminder, len(reminders))
	for _, r := range reminders {
		reminderMap[r.ID] = r
	}

	type chRow struct {
		ReminderID uint   `gorm:"column:reminder_id"`
		ChannelID  uint   `gorm:"column:channel_id"`
		Name       string `gorm:"column:name"`
		Type       string `gorm:"column:type"`
	}
	var chRows []chRow
	if err := e.db.Table("reminder_channels").
		Select("reminder_channels.reminder_id, channels.id AS channel_id, channels.name, channels.type").
		Joins("JOIN channels ON channels.id = reminder_channels.channel_id").
		Where("reminder_channels.reminder_id IN ?", ids).
		Order("channels.name ASC").
		Scan(&chRows).Error; err != nil {
		slog.Error("ListRegistered 查询通道失败", "error", err)
	}

	chMap := make(map[uint][]ChannelInfo)
	for _, row := range chRows {
		chMap[row.ReminderID] = append(chMap[row.ReminderID], ChannelInfo{
			ID:   row.ChannelID,
			Name: row.Name,
			Type: row.Type,
		})
	}

	for id, r := range e.reg {
		ent := RegisteredEntry{
			ID:       id,
			Kind:     string(r.kind),
			Channels: chMap[id],
		}
		if ent.Channels == nil {
			ent.Channels = []ChannelInfo{}
		}
		if rem, ok := reminderMap[id]; ok {
			ent.NextFireAt = rem.NextFireAt
			ent.Title = rem.Title
			ent.LastFiredAt = rem.LastFiredAt
			ent.ScheduleType = rem.ScheduleType
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
		if all[i].NextFireAt.Equal(all[j].NextFireAt.Std()) {
			return all[i].ID < all[j].ID
		}
		return all[i].NextFireAt.Before(all[j].NextFireAt.Std())
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

// fire 是所有触发器的统一入口：重读提醒 → 事务内乐观锁推进 next_fire_at 并插入
// pending 日志 → 异步投递 → 非 cron 触发器注册下一次。
func (e *Engine) fire(id uint) {
	e.lastFireMu.Lock()
	e.lastFireAt = time.Now()
	e.lastFireMu.Unlock()

	var r models.Reminder
	if err := e.db.First(&r, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// 提醒已被删除（或并发删除），调度器里的定时器属于残留，无需记录
			return
		}
		slog.Error("触发时读取提醒失败", "reminder", id, "error", err)
		return
	}
	if !r.Enabled {
		return
	}

	now := time.Now()
	// next_fire_at 仍在未来（留 2 秒时钟抖动的容差）说明已被其它路径触发并推进
	if r.NextFireAt != nil && r.NextFireAt.After(now.Add(2*time.Second)) {
		return
	}

	spec, err := ParseSpec(r.ScheduleSpec)
	if err != nil {
		slog.Error("触发时解析 schedule_spec 失败，本次触发被跳过", "reminder", id, "title", r.Title, "spec", string(r.ScheduleSpec), "error", err)
		return
	}

	// 算出新的 next_fire_at（cron 也要算，用于持久化展示）
	newNext, err := Compute(r.Calendar, r.ScheduleType, spec, now, e.tz.Get())
	if err != nil {
		// 本次照常投递，但 next_fire_at 会写空，必须留痕
		slog.Error("触发时计算下次触发时间失败，本次仍投递，next_fire_at 将写空",
			"reminder", id, "title", r.Title, "schedule_type", r.ScheduleType, "calendar", r.Calendar, "error", err)
	}
	// 统一以 UTC 时间落库
	nowUTC := models.TimeOf(now)
	nextStored := models.TimePtr(newNext)

	var logID uint
	err = e.db.Transaction(func(tx *gorm.DB) error {
		// 乐观锁：用旧 next_fire_at 做等值条件，防止重复触发
		updates := map[string]any{
			"last_fired_at": nowUTC,
			"fire_count":    gorm.Expr("fire_count + 1"),
		}
		q := tx.Model(&models.Reminder{}).Where("id = ? AND enabled = ?", id, true)
		ok, err := advanceReminder(q, &r, nextStored, updates)
		if err != nil {
			return err
		}
		if !ok {
			// 别的路径（sweeper 或并发触发）已经处理过这条，跳过
			return errSkipped
		}
		// 插入 pending 日志
		dlog := newDeliveryLog(&r, "pending", nowUTC, 0)
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
		slog.Error("触发事务失败，本次投递未发生", "reminder", id, "title", r.Title, "error", err)
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
				slog.Error("触发后重新注册下一次失败，该提醒将不再自动触发（依赖兜底扫描）",
					"reminder", id, "title", r.Title, "error", err)
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
			slog.Error("注册提醒到调度器失败，该提醒不会被自动触发（依赖兜底扫描）",
				"reminder", items[i].ID, "title", items[i].Title,
				"schedule_type", items[i].ScheduleType, "calendar", items[i].Calendar,
				"spec", string(items[i].ScheduleSpec), "error", err)
		}
	}
	return nil
}

// AddPurgeCron 注册一个每天凌晨 3 点的日志清理任务（ReloadTimezone 后自动恢复）。
func (e *Engine) AddPurgeCron(logSrv interface {
	Purge(olderThan time.Duration, all bool) (int64, error)
}, purgeAfter time.Duration) (cron.EntryID, error) {
	job := func() {
		n, err := logSrv.Purge(purgeAfter, false)
		if err != nil {
			slog.Error("自动清理日志失败", "error", err)
		} else if n > 0 {
			slog.Info("自动清理日志", "count", n)
		}
	}
	return e.AddDailyJob("日志清理", DailyPurgeExpr, job)
}

// DailyPurgeExpr 是维护型任务的运行时刻：每天凌晨 3 点（含秒字段）。
const DailyPurgeExpr = "0 0 3 * * *"

// AddDailyJob 注册一个每天凌晨 3 点运行的维护型任务；name 只用于日志。
func (e *Engine) AddDailyJob(name string, expr string, job func()) (cron.EntryID, error) {
	wrapped := func() {
		defer func() {
			// 捕获并记录维护任务 panic
			if r := recover(); r != nil {
				slog.Error("维护任务 panic", "job", name, "recover", r, "stack", debug.Stack())
			}
		}()
		job()
	}
	// 加锁登记与注册 cron 任务
	e.mu.Lock()
	defer e.mu.Unlock()
	id, err := e.cron.AddFunc(expr, wrapped)
	if err != nil {
		return id, err
	}
	e.purgeJobs = append(e.purgeJobs, purgeJob{expr: expr, job: wrapped})
	return id, nil
}
