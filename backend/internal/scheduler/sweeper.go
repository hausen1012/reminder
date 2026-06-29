// Sweeper 是调度器的兜底扫描器：
//   - 进程崩溃 / 重启窗口期内可能错过的触发，由 sweeper 在容忍窗口内补发；
//   - 超出 MISS_TOLERANCE_MINUTES 的过期触发，直接标记 expired 并向前推进 next_fire_at。
//
// 与 Engine.fire 共享同一套乐观锁，所以二者并发不会重复触发。
package scheduler

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/reminder/backend/internal/models"
	"gorm.io/gorm"
)

// Sweeper 后台定时扫描 reminders 表，补漏触发或标记过期。
type Sweeper struct {
	db          *gorm.DB
	engine      *Engine
	loc         *time.Location
	interval    time.Duration
	missTol     time.Duration
	skewSeconds time.Duration

	cancel context.CancelFunc
	done   chan struct{}
}

// NewSweeper 构造一个 Sweeper。
//
// interval 决定扫描间隔；missTol 决定漏触发的容忍上限，超过即标记 expired。
// skewSeconds 用来避免与正常触发器抢同一秒，建议给 30s 以上。
func NewSweeper(db *gorm.DB, engine *Engine, loc *time.Location, interval, missTol, skew time.Duration) *Sweeper {
	if loc == nil {
		loc = time.Local
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
		loc:         loc,
		interval:    interval,
		missTol:     missTol,
		skewSeconds: skew,
		done:        make(chan struct{}),
	}
}

// Start 启动后台 goroutine 周期扫描，可重复调用幂等。
func (s *Sweeper) Start() {
	if s.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	go s.loop(ctx)
}

// IsRunning 返回 sweeper 是否在运行。
func (s *Sweeper) IsRunning() bool {
	return s.cancel != nil
}

// Interval 返回扫描间隔。
func (s *Sweeper) Interval() time.Duration {
	return s.interval
}

// Stop 通知 sweeper 退出，并等待清理完成。
func (s *Sweeper) Stop() {
	if s.cancel == nil {
		return
	}
	s.cancel()
	<-s.done
	s.cancel = nil
}

func (s *Sweeper) loop(ctx context.Context) {
	defer close(s.done)
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
	cutoffDue := now.Add(-s.skewSeconds)
	cutoffExpire := now.Add(-s.missTol)

	var items []models.Reminder
	err := s.db.
		Where("enabled = ? AND next_fire_at IS NOT NULL AND next_fire_at <= ?", true, cutoffDue).
		Find(&items).Error
	if err != nil {
		slog.Info("扫描失败", "error", err)
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
						slog.Info("标记 reminder expired 失败", "reminder", r.ID, "error", err)
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
	newNext, err := Compute(r.Calendar, r.ScheduleType, spec, now, s.loc)
	if err != nil {
		return err
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		oldNext := r.NextFireAt
		q := tx.Model(&models.Reminder{}).Where("id = ?", r.ID)
		if oldNext != nil {
			q = q.Where("next_fire_at = ?", oldNext)
		} else {
			q = q.Where("next_fire_at IS NULL")
		}
		updates := map[string]any{
			"next_fire_at": newNext,
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

// 让 sweeper 在调度器停止时也能感知到（目前仅为可读性占位）。
var _ = context.Canceled
