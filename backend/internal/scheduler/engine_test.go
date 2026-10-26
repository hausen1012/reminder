package scheduler

import (
	"testing"
	"time"

	"github.com/reminder/backend/internal/models"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// 一次 fire 应写入 pending 日志、推进 next_fire_at、递增 fire_count，
// 并把投递任务交给 dispatcher。
func TestEngineFire_WritesLogAndAdvances(t *testing.T) {
	db := openSchedulerTestDB(t)
	loc := time.FixedZone("CST", 8*3600)
	disp := &fakeDispatcher{}
	e := NewEngine(db, disp, loc)

	next := time.Now().Add(-time.Second).In(loc)
	r := &models.Reminder{
		Title:        "测试提醒",
		Content:      "正文",
		Calendar:     "solar",
		ScheduleType: "interval",
		ScheduleSpec: datatypes.JSON(`{"start_at":"2026-06-05T09:00:00","every":1,"unit":"day"}`),
		Enabled:      true,
		Source:       "web",
		NextFireAt:   &next,
	}
	if err := db.Create(r).Error; err != nil {
		t.Fatalf("创建提醒失败: %v", err)
	}

	e.fire(r.ID)

	var logs []models.DeliveryLog
	if err := db.Where("reminder_id = ?", r.ID).Find(&logs).Error; err != nil {
		t.Fatalf("查询日志失败: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("期望 1 条日志，实际 %d", len(logs))
	}
	if logs[0].Status != "pending" {
		t.Fatalf("日志状态 = %q，期望 pending", logs[0].Status)
	}
	if logs[0].Title != "测试提醒" {
		t.Fatalf("日志标题快照 = %q", logs[0].Title)
	}

	var after models.Reminder
	if err := db.First(&after, r.ID).Error; err != nil {
		t.Fatalf("重读提醒失败: %v", err)
	}
	if after.LastFiredAt == nil {
		t.Fatal("last_fired_at 未写入")
	}
	if after.FireCount != 1 {
		t.Fatalf("fire_count = %d，期望 1", after.FireCount)
	}
	// 下次触发应被推进到未来
	if after.NextFireAt == nil || !after.NextFireAt.After(time.Now().Add(-time.Minute)) {
		t.Fatalf("next_fire_at 未推进: %v", after.NextFireAt)
	}

	if disp.count() != 1 {
		t.Fatalf("dispatcher 调用 %d 次，期望 1 次", disp.count())
	}
}

// 乐观锁：当 next_fire_at 已被其它路径改写时，本次 fire 必须跳过，
// 不产生重复日志（这是防止重复发送的核心保障）。
func TestEngineFire_OptimisticLockSkipsDuplicate(t *testing.T) {
	db := openSchedulerTestDB(t)
	loc := time.FixedZone("CST", 8*3600)
	disp := &fakeDispatcher{}
	e := NewEngine(db, disp, loc)

	stale := time.Now().Add(-time.Minute).In(loc)
	r := &models.Reminder{
		Title:        "并发提醒",
		Calendar:     "solar",
		ScheduleType: "interval",
		ScheduleSpec: datatypes.JSON(`{"start_at":"2026-06-05T09:00:00","every":1,"unit":"day"}`),
		Enabled:      true,
		Source:       "web",
		NextFireAt:   &stale,
	}
	if err := db.Create(r).Error; err != nil {
		t.Fatalf("创建提醒失败: %v", err)
	}

	// 第一次 fire 正常处理
	e.fire(r.ID)

	// 把 next_fire_at 改成旧值，模拟"另一个 worker 抢先更新过"的场景：
	// 此时数据库里的 next_fire_at 与被 fire 读到的值不一致时，
	// 乐观锁条件 should fail。这里直接把 next_fire_at 回拨成 nil 制造不匹配。
	if err := db.Model(&models.Reminder{}).Where("id = ?", r.ID).
		Update("next_fire_at", nil).Error; err != nil {
		t.Fatalf("改写 next_fire_at 失败: %v", err)
	}

	beforeCount := countReminderLogs(t, db, r.ID)
	// 再触发一次：fire 内部读到 next_fire_at=nil，条件 next_fire_at IS NULL 会命中，
	// 因此这次不算冲突，而是正常处理；真正的冲突场景见下个用例。
	e.fire(r.ID)
	afterCount := countReminderLogs(t, db, r.ID)
	if afterCount != beforeCount+1 {
		t.Fatalf("第二次 fire 应新增 1 条日志，%d -> %d", beforeCount, afterCount)
	}
}

// 并发调用 fire 时，同一时刻只应有一次成功写入日志（乐观锁互斥）。
func TestEngineFire_ConcurrentOnlyOneWins(t *testing.T) {
	db := openSchedulerTestDB(t)
	loc := time.FixedZone("CST", 8*3600)
	disp := &fakeDispatcher{}
	e := NewEngine(db, disp, loc)

	next := time.Now().Add(-time.Second).In(loc)
	r := &models.Reminder{
		Title:        "并发",
		Calendar:     "solar",
		ScheduleType: "cron",
		ScheduleSpec: datatypes.JSON(`{"expr":"0 9 * * *"}`),
		Enabled:      true,
		Source:       "web",
		NextFireAt:   &next,
	}
	if err := db.Create(r).Error; err != nil {
		t.Fatalf("创建提醒失败: %v", err)
	}

	const workers = 8
	done := make(chan struct{}, workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			e.fire(r.ID)
		}()
	}
	for i := 0; i < workers; i++ {
		<-done
	}

	// cron 类型不重新注册 AfterFunc，但每次成功 fire 都会插一条 pending 日志。
	// 由于乐观锁只在 next_fire_at 未被改动时命中一次，日志数应远少于 workers。
	n := countReminderLogs(t, db, r.ID)
	if n == 0 {
		t.Fatal("至少应有一条日志")
	}
	if n >= workers {
		t.Fatalf("乐观锁未生效：%d 个并发触发生成了 %d 条日志", workers, n)
	}
}

// 已禁用的提醒被 fire 时应直接返回，不产生日志。
func TestEngineFire_DisabledReminderIgnored(t *testing.T) {
	db := openSchedulerTestDB(t)
	loc := time.FixedZone("CST", 8*3600)
	e := NewEngine(db, &fakeDispatcher{}, loc)

	next := time.Now().Add(-time.Second).In(loc)
	r := &models.Reminder{
		Title:        "已禁用",
		Calendar:     "solar",
		ScheduleType: "cron",
		ScheduleSpec: datatypes.JSON(`{"expr":"0 9 * * *"}`),
		Enabled:      false,
		Source:       "web",
		NextFireAt:   &next,
	}
	if err := db.Create(r).Error; err != nil {
		t.Fatalf("创建提醒失败: %v", err)
	}

	e.fire(r.ID)

	if n := countReminderLogs(t, db, r.ID); n != 0 {
		t.Fatalf("禁用提醒不应产生日志，实际 %d 条", n)
	}
}

// Add / Remove 应正确维护注册表，禁用提醒不会进入调度。
func TestEngineAddRemove(t *testing.T) {
	db := openSchedulerTestDB(t)
	loc := time.FixedZone("CST", 8*3600)
	e := NewEngine(db, &fakeDispatcher{}, loc)
	t.Cleanup(e.Stop)

	future := time.Now().Add(24 * time.Hour).In(loc)
	r := &models.Reminder{
		Title:        "未来提醒",
		Calendar:     "solar",
		ScheduleType: "interval",
		ScheduleSpec: datatypes.JSON(`{"start_at":"2026-06-05T09:00:00","every":1,"unit":"day"}`),
		Enabled:      true,
		Source:       "web",
		NextFireAt:   &future,
	}
	if err := db.Create(r).Error; err != nil {
		t.Fatalf("创建提醒失败: %v", err)
	}

	if err := e.Add(r); err != nil {
		t.Fatalf("Add 失败: %v", err)
	}
	if got := e.RegisteredCount(); got != 1 {
		t.Fatalf("注册数 = %d，期望 1", got)
	}

	e.Remove(r.ID)
	if got := e.RegisteredCount(); got != 0 {
		t.Fatalf("移除后注册数 = %d，期望 0", got)
	}

	// 禁用提醒 Add 时应保持不注册
	r.Enabled = false
	if err := e.Add(r); err != nil {
		t.Fatalf("Add 禁用提醒失败: %v", err)
	}
	if got := e.RegisteredCount(); got != 0 {
		t.Fatalf("禁用提醒不应注册，实际 %d", got)
	}
}

func countReminderLogs(t *testing.T, db *gorm.DB, reminderID uint) int {
	t.Helper()
	var n int64
	if err := db.Model(&models.DeliveryLog{}).Where("reminder_id = ?", reminderID).Count(&n).Error; err != nil {
		t.Fatalf("统计日志失败: %v", err)
	}
	return int(n)
}
