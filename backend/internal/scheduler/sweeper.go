// Sweeper 是调度器的兜底扫描器：
//   - 进程崩溃 / 重启窗口期内可能错过的触发，由 sweeper 在容忍窗口内补发；
//   - 超出 MISS_TOLERANCE_MINUTES 的过期触发，直接标记 expired 并向前推进 next_fire_at；
//   - 卡在 pending 的投递日志（进程在投递中途退出）被收敛为 failed。
//
// 与 Engine.fire 共享同一套乐观锁，所以二者并发不会重复触发。
//
// 注意：next_fire_at 是 models.Time（Unix 毫秒整数），比较与排序都是整数语义，
// 写入时的时区偏移不会影响结果。
package scheduler

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/reminder/backend/internal/models"
	"github.com/reminder/backend/internal/timezone"
	"gorm.io/gorm"
)

// Sweeper 后台定时扫描 reminders 表，补漏触发或标记过期。
type Sweeper struct {
	db          *gorm.DB
	engine      *Engine
	tz          *timezone.Provider
	interval    time.Duration
	missTol     time.Duration
	skewSeconds time.Duration
	// pendingTTL 是 pending 日志的存活上限：超过它仍是 pending，
	// 说明投递 goroutine 已经消失（进程退出 / 被强制杀死），不会再有人推进终态。
	pendingTTL time.Duration

	// mu 保护下面两个字段：IsRunning 会被 HTTP 请求 goroutine 调用，
	// 而 Start/Stop 来自 main 的启动与停机流程，不加锁读是数据竞争。
	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{} // 与 cancel 一同在 Start 里重建，Stop 靠它等待 goroutine 退出
}

// pendingTTL 是 pending 日志的收敛阈值。
//
// 单个通道最长重试链是「立即 + 10s + 30s」，加上三次请求各自的超时
// （Webhook 15s、SMTP 15s），单通道最坏约 1 分钟；多个通道并发。
// 取 10 分钟是相对宽松的上限：正常投递不可能跑这么久，
// 而太短会把仍在重试的投递误判为失败。
const pendingTTL = 10 * time.Minute

// NewSweeper 构造一个 Sweeper。
//
// interval 决定扫描间隔；missTol 决定漏触发的容忍上限，超过即标记 expired。
// skewSeconds 用来避免与正常触发器抢同一秒，建议给 30s 以上。
// tz 为空时回退到默认站点时区。
func NewSweeper(db *gorm.DB, engine *Engine, tz *timezone.Provider, interval, missTol, skew time.Duration) *Sweeper {
	if tz == nil {
		tz = timezone.New("")
	}
	if interval <= 0 {
		interval = 60 * time.Second
	}
	if missTol <= 0 {
		missTol = 60 * time.Minute
	}
	if skew <= 0 {
		skew = 30 * time.Second
	}
	return &Sweeper{
		db:          db,
		engine:      engine,
		tz:          tz,
		interval:    interval,
		missTol:     missTol,
		skewSeconds: skew,
		pendingTTL:  pendingTTL,
	}
}

// Start 启动后台 goroutine 周期扫描，可重复调用幂等。
func (s *Sweeper) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	// done 与 cancel 成对重建：Stop 之后再次 Start 需要一个新的信号 channel，
	// 复用旧 channel（已 close）会让新 goroutine 的 defer close 直接 panic。
	s.done = make(chan struct{})
	s.cancel = cancel
	go s.loop(ctx, s.done)
}

// IsRunning 返回 sweeper 是否在运行。
//
// 加锁读：/api/health 与 /api/scheduler/status 会在请求 goroutine 里调它，
// 而 Start/Stop 可能来自 main 的启动与停机流程，无锁读 cancel 是数据竞争。
func (s *Sweeper) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cancel != nil
}

// Interval 返回扫描间隔。
func (s *Sweeper) Interval() time.Duration {
	return s.interval
}

// Stop 通知 sweeper 退出，并等待清理完成。
//
// 先把 cancel/done 摘下来再释放锁，然后才等待 goroutine 退出：
// 若持锁等待，正在执行的 tick 可能卡住；若等完再清空 s.cancel，
// 并发 Start 新建的 cancel 会被误清。
func (s *Sweeper) Stop() {
	s.mu.Lock()
	cancel, done := s.cancel, s.done
	s.cancel, s.done = nil, nil
	s.mu.Unlock()
	if cancel == nil {
		return
	}
	cancel()
	<-done
}

func (s *Sweeper) loop(ctx context.Context, done chan struct{}) {
	defer close(done)
	t := time.NewTicker(s.interval)
	defer t.Stop()
	// 启动后先立即跑一次，便于重启时尽快补漏
	s.tick()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.tick()
		}
	}
}

// tick 是一次完整的扫描。
//
// 步骤：
//  1. 查 enabled=true AND next_fire_at IS NOT NULL AND next_fire_at <= now - skew
//  2. 对每条：
//     - 若 next_fire_at >= now - missTol：调 engine.fire（带乐观锁，不重复触发）
//     - 否则：在事务里写一条 expired 日志并把 next_fire_at 推进
func (s *Sweeper) tick() {
	now := time.Now()
	// 先收敛卡在 pending 的日志，再做提醒补漏：
	// 两者互不依赖，但先处理日志能让本轮日志统计立刻是准确的。
	s.recoverStalePendingLogs(now)

	// 边界值统一取 UTC，与库里的整数毫秒按同一时刻比较
	cutoffDue := now.Add(-s.skewSeconds)
	cutoffExpire := now.Add(-s.missTol)

	var items []models.Reminder
	err := s.db.
		Where("enabled = ? AND next_fire_at IS NOT NULL AND next_fire_at <= ?", true, models.TimeOf(cutoffDue)).
		Find(&items).Error
	if err != nil {
		slog.Error("兜底扫描查询失败，本轮补漏跳过", "error", err)
		return
	}

	// 用 worker pool 并发处理，避免大量过期提醒串行阻塞
	workerCount := 4
	if n := len(items); n < workerCount {
		workerCount = n
	}
	ch := make(chan *models.Reminder, len(items))
	var wg sync.WaitGroup
	for w := 0; w < workerCount; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for r := range ch {
				if r.NextFireAt == nil {
					continue
				}
				if r.NextFireAt.Before(cutoffExpire) {
					if err := s.markExpired(r, now); err != nil {
						slog.Error("标记提醒为 expired 失败", "reminder", r.ID, "title", r.Title, "error", err)
					}
					continue
				}
				// 在 missTol 内：交给 engine.fire 走正常路径
				s.engine.fire(r.ID)
			}
		}()
	}
	for i := range items {
		ch <- &items[i]
	}
	close(ch)
	wg.Wait()
}

// markExpired 把一条已严重过期的提醒推进到下一次：
//   - 写一条 status=expired 的 delivery_log（便于用户排查）
//   - next_fire_at 推进到 Compute(now)（cron/interval）；once 直接置 nil 并 disable
//
// 使用与 engine.fire 一致的乐观锁，避免与正常触发器抢更新。
func (s *Sweeper) markExpired(r *models.Reminder, now time.Time) error {
	spec, err := ParseSpec(r.ScheduleSpec)
	if err != nil {
		return err
	}
	newNext, err := Compute(r.Calendar, r.ScheduleType, spec, now, s.tz.Get())
	if err != nil {
		return err
	}
	nextStored := models.TimePtr(newNext)

	return s.db.Transaction(func(tx *gorm.DB) error {
		oldNext := r.NextFireAt
		q := tx.Model(&models.Reminder{}).Where("id = ?", r.ID)
		if oldNext != nil {
			// 同 engine.fire：整数毫秒列的等值比较是精确的
			q = q.Where("next_fire_at = ?", *oldNext)
		} else {
			q = q.Where("next_fire_at IS NULL")
		}
		updates := map[string]any{
			"next_fire_at": nextStored,
		}
		if r.ScheduleType == "once" && newNext == nil {
			// 单次已过期：禁用，避免下次启动再扫到
			updates["enabled"] = false
		}
		res := q.Updates(updates)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			// 别人改过了，跳过
			return errSweepSkipped
		}
		dlog := &models.DeliveryLog{
			ReminderID: r.ID,
			FiredAt:    *r.NextFireAt,
			Title:      r.Title,
			Content:    r.Content,
			Status:     "expired",
			Source:     r.Source,
			RetryRound: 0,
		}
		if err := tx.Create(dlog).Error; err != nil {
			return err
		}
		return nil
	})
}

var errSweepSkipped = errors.New("sweep skipped by optimistic lock")

// recoverStalePendingLogs 把卡在 pending 的投递日志收敛为 failed。
//
// 为什么必须有这一步：Engine.fire 在事务里插入 pending 日志后，投递是异步 goroutine。
// 如果进程恰好在投递中途退出（重启、OOM、被 kill -9），这条日志就永远停在 pending——
// 没有任何代码会再碰它：sweeper 只扫 reminders 表，调度器只处理 next_fire_at。
// 结果是日志页长期挂着「发送中」，成功率统计被永久污染。
//
// 判据是 fired_at 早于 now - pendingTTL，而不是「启动时」：
// 进程正常运行期间也可能出现 goroutine 意外消失（panic 未被 recover 到），
// 用时间窗口判定可以覆盖这两种情况，且不需要额外的启动钩子。
//
// 同时写一条日志：这不是正常业务结果，运维需要知道发生过。
func (s *Sweeper) recoverStalePendingLogs(now time.Time) {
	cutoff := models.TimeOf(now.Add(-s.pendingTTL))
	res := s.db.Model(&models.DeliveryLog{}).
		Where("status = ? AND fired_at < ?", "pending", cutoff).
		Updates(map[string]any{
			"status": "failed",
			"error":  "投递未完成（进程可能中途退出），已由兜底扫描标记为失败",
		})
	if res.Error != nil {
		slog.Error("收敛卡住的 pending 日志失败", "error", res.Error)
		return
	}
	if res.RowsAffected > 0 {
		slog.Warn("发现卡在 pending 的投递日志，已标记为 failed",
			"count", res.RowsAffected, "older_than", s.pendingTTL)
	}
}
