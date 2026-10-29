package scheduler

import (
	"sync"
	"testing"
	"time"

	"github.com/reminder/backend/internal/models"
	"gorm.io/datatypes"
)

// 漏触发在容忍窗口内：应补发（产生 pending 日志），而标记为 expired。
func TestSweeperTick_FiresWithinTolerance(t *testing.T) {
	db := openSchedulerTestDB(t)
	loc := time.FixedZone("CST", 8*3600)
	disp := &fakeDispatcher{}
	engine := NewEngine(db, disp, testTZ(loc))
	t.Cleanup(engine.Stop)

	// 10 分钟前触发，容忍窗口 60 分钟 → 应补发
	missed := time.Now().Add(-10 * time.Minute).In(loc)
	r := &models.Reminder{
		Title:        "漏触发补发",
		Calendar:     "solar",
		ScheduleType: "interval",
		ScheduleSpec: datatypes.JSON(`{"start_at":"2026-06-05T09:00:00","every":1,"unit":"day"}`),
		Enabled:      true,
		Source:       "web",
		NextFireAt:   models.TimePtrOf(missed),
	}
	if err := db.Create(r).Error; err != nil {
		t.Fatalf("创建提醒失败: %v", err)
	}

	s := NewSweeper(db, engine, testTZ(loc), time.Minute, 60*time.Minute, 0)
	s.tick()

	var logs []models.DeliveryLog
	if err := db.Where("reminder_id = ?", r.ID).Find(&logs).Error; err != nil {
		t.Fatalf("查询日志失败: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("期望 1 条日志，实际 %d", len(logs))
	}
	if logs[0].Status != "pending" {
		t.Fatalf("容忍窗口内应补发（pending），实际状态 %q", logs[0].Status)
	}
}

// 漏触发超出容忍窗口：应标记 expired，不再发送。
func TestSweeperTick_MarksExpiredBeyondTolerance(t *testing.T) {
	db := openSchedulerTestDB(t)
	loc := time.FixedZone("CST", 8*3600)
	engine := NewEngine(db, &fakeDispatcher{}, testTZ(loc))
	t.Cleanup(engine.Stop)

	// 2 小时前触发，容忍窗口 60 分钟 → 应标记 expired
	missed := time.Now().Add(-2 * time.Hour).In(loc)
	r := &models.Reminder{
		Title:        "严重过期",
		Calendar:     "solar",
		ScheduleType: "interval",
		ScheduleSpec: datatypes.JSON(`{"start_at":"2026-06-05T09:00:00","every":1,"unit":"day"}`),
		Enabled:      true,
		Source:       "web",
		NextFireAt:   models.TimePtrOf(missed),
	}
	if err := db.Create(r).Error; err != nil {
		t.Fatalf("创建提醒失败: %v", err)
	}

	s := NewSweeper(db, engine, testTZ(loc), time.Minute, 60*time.Minute, 0)
	s.tick()

	var logs []models.DeliveryLog
	if err := db.Where("reminder_id = ?", r.ID).Find(&logs).Error; err != nil {
		t.Fatalf("查询日志失败: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("期望 1 条 expired 日志，实际 %d 条", len(logs))
	}
	if logs[0].Status != "expired" {
		t.Fatalf("状态 = %q，期望 expired", logs[0].Status)
	}

	// next_fire_at 应被推进到未来，避免下次启动再扫到同一条
	var after models.Reminder
	if err := db.First(&after, r.ID).Error; err != nil {
		t.Fatalf("重读提醒失败: %v", err)
	}
	if after.NextFireAt != nil && !after.NextFireAt.After(time.Now()) {
		t.Fatalf("next_fire_at 未推进: %v", after.NextFireAt)
	}
}

// 单次提醒严重过期：应置 nil 并禁用，避免下次启动再扫到。
func TestSweeperTick_OnceExpiredDisables(t *testing.T) {
	db := openSchedulerTestDB(t)
	loc := time.FixedZone("CST", 8*3600)
	engine := NewEngine(db, &fakeDispatcher{}, testTZ(loc))
	t.Cleanup(engine.Stop)

	missed := time.Now().Add(-48 * time.Hour).In(loc)
	r := &models.Reminder{
		Title:        "一次性过期",
		Calendar:     "solar",
		ScheduleType: "once",
		ScheduleSpec: datatypes.JSON(`{"at":"2020-01-01T09:00:00"}`),
		Enabled:      true,
		Source:       "web",
		NextFireAt:   models.TimePtrOf(missed),
	}
	if err := db.Create(r).Error; err != nil {
		t.Fatalf("创建提醒失败: %v", err)
	}

	s := NewSweeper(db, engine, testTZ(loc), time.Minute, 60*time.Minute, 0)
	s.tick()

	var after models.Reminder
	if err := db.First(&after, r.ID).Error; err != nil {
		t.Fatalf("重读提醒失败: %v", err)
	}
	if after.Enabled {
		t.Fatal("一次性提醒严重过期后应被禁用")
	}
	if after.NextFireAt != nil {
		t.Fatalf("next_fire_at 应被置 nil，实际 %v", after.NextFireAt)
	}
}

// 未到期的提醒不应被 sweeper 处理。
func TestSweeperTick_IgnoresFutureReminders(t *testing.T) {
	db := openSchedulerTestDB(t)
	loc := time.FixedZone("CST", 8*3600)
	engine := NewEngine(db, &fakeDispatcher{}, testTZ(loc))
	t.Cleanup(engine.Stop)

	future := time.Now().Add(time.Hour).In(loc)
	r := &models.Reminder{
		Title:        "未到期",
		Calendar:     "solar",
		ScheduleType: "interval",
		ScheduleSpec: datatypes.JSON(`{"start_at":"2026-06-05T09:00:00","every":1,"unit":"day"}`),
		Enabled:      true,
		Source:       "web",
		NextFireAt:   models.TimePtrOf(future),
	}
	if err := db.Create(r).Error; err != nil {
		t.Fatalf("创建提醒失败: %v", err)
	}

	s := NewSweeper(db, engine, testTZ(loc), time.Minute, 60*time.Minute, 0)
	s.tick()

	if n := countReminderLogs(t, db, r.ID); n != 0 {
		t.Fatalf("未到期提醒不应产生日志，实际 %d 条", n)
	}
}

// Sweeper 生命周期：Start → IsRunning → Interval → Stop。
func TestSweeperStartStop(t *testing.T) {
	db := openSchedulerTestDB(t)
	loc := time.FixedZone("CST", 8*3600)
	engine := NewEngine(db, &fakeDispatcher{}, testTZ(loc))
	t.Cleanup(engine.Stop)

	s := NewSweeper(db, engine, testTZ(loc), 100*time.Millisecond, time.Hour, 30*time.Second)

	if s.IsRunning() {
		t.Fatal("未 Start 前不应 running")
	}

	s.Start()
	if !s.IsRunning() {
		t.Fatal("Start 后应为 running")
	}
	if got := s.Interval(); got != 100*time.Millisecond {
		t.Fatalf("Interval = %v", got)
	}

	// 重复 Start 应幂等
	s.Start()
	if !s.IsRunning() {
		t.Fatal("重复 Start 后仍应 running")
	}

	s.Stop()
	if s.IsRunning() {
		t.Fatal("Stop 后不应 running")
	}

	// 重复 Stop 不应 panic
	s.Stop()
}

// NewSweeper 对非法参数应使用默认值。
func TestNewSweeper_Defaults(t *testing.T) {
	db := openSchedulerTestDB(t)
	engine := NewEngine(db, &fakeDispatcher{}, nil)

	s := NewSweeper(db, engine, nil, 0, 0, 0)
	if s.Interval() != 60*time.Second {
		t.Fatalf("interval 默认值 = %v，期望 60s", s.Interval())
	}
	if s.missTol != 60*time.Minute {
		t.Fatalf("missTol 默认值 = %v，期望 60m", s.missTol)
	}
	if s.skewSeconds != 30*time.Second {
		t.Fatalf("skew 默认值 = %v，期望 30s", s.skewSeconds)
	}
}

// loop 应按 interval 周期执行 tick（用短 interval 观察）。
func TestSweeperLoop_RunsPeriodically(t *testing.T) {
	db := openSchedulerTestDB(t)
	loc := time.FixedZone("CST", 8*3600)
	engine := NewEngine(db, &fakeDispatcher{}, testTZ(loc))
	t.Cleanup(engine.Stop)

	// 插入一条严重过期的提醒，sweeper 第一次 tick 就会处理它
	missed := time.Now().Add(-48 * time.Hour).In(loc)
	r := &models.Reminder{
		Title:        "会被 sweep",
		Calendar:     "solar",
		ScheduleType: "once",
		ScheduleSpec: datatypes.JSON(`{"at":"2020-01-01T09:00:00"}`),
		Enabled:      true,
		Source:       "web",
		NextFireAt:   models.TimePtrOf(missed),
	}
	if err := db.Create(r).Error; err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	s := NewSweeper(db, engine, testTZ(loc), 20*time.Millisecond, time.Hour, 0)
	s.Start()
	defer s.Stop()

	// 等待第一次 tick 生效
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var after models.Reminder
		if err := db.First(&after, r.ID).Error; err == nil && !after.Enabled {
			return // 成功：已被标记过期并禁用
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("sweeper loop 未在预期时间内处理过期提醒")
}

// IsRunning / Start / Stop 会在请求 goroutine 与 main 的启停流程里并发访问，
// 本用例在 -race 下验证没有数据竞争。
func TestSweeperConcurrentStatusAccess(t *testing.T) {
	db := openSchedulerTestDB(t)
	loc := time.FixedZone("CST", 8*3600)
	engine := NewEngine(db, &fakeDispatcher{}, testTZ(loc))
	t.Cleanup(engine.Stop)

	// 扫描间隔给到 1 小时：测试期间不会真的扫库
	s := NewSweeper(db, engine, testTZ(loc), time.Hour, time.Hour, time.Hour)

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// 一组 goroutine 持续读状态（模拟 /api/health 与监控页轮询）
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					_ = s.IsRunning()
					_ = s.Interval()
				}
			}
		}()
	}

	// 另一组反复启停
	for i := 0; i < 3; i++ {
		s.Start()
		s.Stop()
	}

	close(stop)
	wg.Wait()

	if s.IsRunning() {
		t.Fatal("全部 Stop 之后不应 running")
	}
}

// 并发 Start 只应产生一个后台 goroutine：重复 Start 必须幂等。
func TestSweeperStartIdempotentUnderConcurrency(t *testing.T) {
	db := openSchedulerTestDB(t)
	loc := time.FixedZone("CST", 8*3600)
	engine := NewEngine(db, &fakeDispatcher{}, testTZ(loc))
	t.Cleanup(engine.Stop)

	s := NewSweeper(db, engine, testTZ(loc), time.Hour, time.Hour, time.Hour)
	t.Cleanup(s.Stop)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.Start()
		}()
	}
	wg.Wait()

	if !s.IsRunning() {
		t.Fatal("并发 Start 后应为 running")
	}
}
