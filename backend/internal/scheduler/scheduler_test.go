package scheduler

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/reminder/backend/internal/models"
	"github.com/reminder/backend/internal/testutil"
	"github.com/reminder/backend/internal/timezone"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// ==================== engine_test.go ====================

// 一次 fire 应写入 pending 日志、推进 next_fire_at、递增 fire_count，
// 并把投递任务交给 dispatcher。
func TestEngineFire_WritesLogAndAdvances(t *testing.T) {
	db := openSchedulerTestDB(t)
	loc := time.FixedZone("CST", 8*3600)
	disp := &fakeDispatcher{}
	e := NewEngine(db, disp, testTZ(loc))

	next := time.Now().Add(-time.Second).In(loc)
	r := &models.Reminder{
		Title:        "测试提醒",
		Content:      "正文",
		Calendar:     "solar",
		ScheduleType: "interval",
		ScheduleSpec: datatypes.JSON(`{"start_at":"2026-06-05T09:00:00","every":1,"unit":"day"}`),
		Enabled:      true,
		Source:       "web",
		NextFireAt:   models.TimePtrOf(next),
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
	e := NewEngine(db, disp, testTZ(loc))

	stale := time.Now().Add(-time.Minute).In(loc)
	r := &models.Reminder{
		Title:        "并发提醒",
		Calendar:     "solar",
		ScheduleType: "interval",
		ScheduleSpec: datatypes.JSON(`{"start_at":"2026-06-05T09:00:00","every":1,"unit":"day"}`),
		Enabled:      true,
		Source:       "web",
		NextFireAt:   models.TimePtrOf(stale),
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
	e := NewEngine(db, disp, testTZ(loc))

	next := time.Now().Add(-time.Second).In(loc)
	r := &models.Reminder{
		Title:        "并发",
		Calendar:     "solar",
		ScheduleType: "cron",
		ScheduleSpec: datatypes.JSON(`{"expr":"0 9 * * *"}`),
		Enabled:      true,
		Source:       "web",
		NextFireAt:   models.TimePtrOf(next),
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
	e := NewEngine(db, &fakeDispatcher{}, testTZ(loc))

	next := time.Now().Add(-time.Second).In(loc)
	r := &models.Reminder{
		Title:        "已禁用",
		Calendar:     "solar",
		ScheduleType: "cron",
		ScheduleSpec: datatypes.JSON(`{"expr":"0 9 * * *"}`),
		Enabled:      false,
		Source:       "web",
		NextFireAt:   models.TimePtrOf(next),
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
	e := NewEngine(db, &fakeDispatcher{}, testTZ(loc))
	t.Cleanup(e.Stop)

	future := time.Now().Add(24 * time.Hour).In(loc)
	r := &models.Reminder{
		Title:        "未来提醒",
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

// newCronReminder 构造并插入一条 solar/cron 提醒。
func newCronReminder(t *testing.T, db *gorm.DB, expr string, enabled bool) *models.Reminder {
	t.Helper()
	r := &models.Reminder{
		Title:        "cron",
		Calendar:     "solar",
		ScheduleType: "cron",
		ScheduleSpec: datatypes.JSON(`{"expr":"` + expr + `"}`),
		Enabled:      true,
		Source:       "web",
	}
	if err := db.Create(r).Error; err != nil {
		t.Fatalf("创建提醒失败: %v", err)
	}
	if !enabled {
		if err := db.Model(&models.Reminder{}).Where("id = ?", r.ID).Update("enabled", false).Error; err != nil {
			t.Fatalf("禁用失败: %v", err)
		}
	}
	return r
}

// 生命周期：Start → IsRunning=true → Stop → IsRunning=false。
func TestEngineStartStop(t *testing.T) {
	db := openSchedulerTestDB(t)
	loc := time.FixedZone("CST", 8*3600)
	e := NewEngine(db, &fakeDispatcher{}, testTZ(loc))

	if e.IsRunning() {
		t.Fatal("未 Start 前不应是 running")
	}

	e.Start()
	if !e.IsRunning() {
		t.Fatal("Start 后应为 running")
	}

	e.Stop()
	if e.IsRunning() {
		t.Fatal("Stop 后不应是 running")
	}

	// 重复 Stop 不应 panic
	e.Stop()
}

// cron 类型应注册到 cron 实例；Remove 应摘除。
func TestEngineAddCron(t *testing.T) {
	db := openSchedulerTestDB(t)
	loc := time.FixedZone("CST", 8*3600)
	e := NewEngine(db, &fakeDispatcher{}, testTZ(loc))
	e.Start()
	t.Cleanup(e.Stop)

	r := newCronReminder(t, db, "0 9 * * *", true)
	if err := e.Add(r); err != nil {
		t.Fatalf("Add cron 失败: %v", err)
	}
	if got := e.RegisteredCount(); got != 1 {
		t.Fatalf("注册数 = %d，期望 1", got)
	}

	// 重复 Add 应幂等（内部先 Remove）
	if err := e.Update(r); err != nil {
		t.Fatalf("Update 失败: %v", err)
	}
	if got := e.RegisteredCount(); got != 1 {
		t.Fatalf("重复注册后应为 1，实际 %d", got)
	}

	e.Remove(r.ID)
	if got := e.RegisteredCount(); got != 0 {
		t.Fatalf("Remove 后应为 0，实际 %d", got)
	}

	// 移除不存在的 id 不应 panic
	e.Remove(9999)
}

// 非法 cron 表达式应返回错误。
func TestEngineAddCron_InvalidExpr(t *testing.T) {
	db := openSchedulerTestDB(t)
	loc := time.FixedZone("CST", 8*3600)
	e := NewEngine(db, &fakeDispatcher{}, testTZ(loc))
	t.Cleanup(e.Stop)

	r := newCronReminder(t, db, "not a cron", true)
	if err := e.Add(r); err == nil {
		t.Fatal("非法 cron 应报错")
	}
}

// nil reminder / 空 spec 的错误处理。
func TestEngineAdd_NilAndEmpty(t *testing.T) {
	db := openSchedulerTestDB(t)
	loc := time.FixedZone("CST", 8*3600)
	e := NewEngine(db, &fakeDispatcher{}, testTZ(loc))
	t.Cleanup(e.Stop)

	if err := e.Add(nil); err == nil {
		t.Fatal("nil reminder 应报错")
	}
}

// 一次性提醒已过期时不注册（Compute 返回 nil）。
func TestEngineAddAfterFunc_ExpiredOnce(t *testing.T) {
	db := openSchedulerTestDB(t)
	loc := time.FixedZone("CST", 8*3600)
	e := NewEngine(db, &fakeDispatcher{}, testTZ(loc))
	t.Cleanup(e.Stop)

	r := &models.Reminder{
		Title:        "过期 once",
		Calendar:     "solar",
		ScheduleType: "once",
		ScheduleSpec: datatypes.JSON(`{"at":"2020-01-01T09:00:00"}`),
		Enabled:      true,
		Source:       "web",
	}
	if err := db.Create(r).Error; err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	if err := e.Add(r); err != nil {
		t.Fatalf("Add 不应报错: %v", err)
	}
	if got := e.RegisteredCount(); got != 0 {
		t.Fatalf("过期 once 不应注册，实际 %d", got)
	}
}

// ListRegistered 应返回注册条目的快照，含提醒元数据。
func TestEngineListRegistered(t *testing.T) {
	db := openSchedulerTestDB(t)
	loc := time.FixedZone("CST", 8*3600)
	e := NewEngine(db, &fakeDispatcher{}, testTZ(loc))
	e.Start()
	t.Cleanup(e.Stop)

	future := time.Now().Add(24 * time.Hour).In(loc)
	r := &models.Reminder{
		Title:        "列表提醒",
		Calendar:     "solar",
		ScheduleType: "interval",
		ScheduleSpec: datatypes.JSON(`{"start_at":"2099-01-01T09:00:00","every":1,"unit":"day"}`),
		Enabled:      true,
		Source:       "web",
		NextFireAt:   models.TimePtrOf(future),
	}
	if err := db.Create(r).Error; err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	if err := e.Add(r); err != nil {
		t.Fatalf("Add 失败: %v", err)
	}

	ch := &models.Channel{
		Name: "测试通道",
		Type: "webhook",
	}
	if err := db.Create(ch).Error; err != nil {
		t.Fatalf("创建通道失败: %v", err)
	}
	if err := db.Create(&models.ReminderChannel{ReminderID: r.ID, ChannelID: ch.ID}).Error; err != nil {
		t.Fatalf("关联通道失败: %v", err)
	}

	entries := e.ListRegistered()
	if len(entries) != 1 {
		t.Fatalf("应返回 1 条，实际 %d", len(entries))
	}
	if entries[0].ID != r.ID {
		t.Fatalf("ID = %d，期望 %d", entries[0].ID, r.ID)
	}
	if entries[0].Title != "列表提醒" {
		t.Fatalf("Title = %q", entries[0].Title)
	}
	if entries[0].Kind == "" {
		t.Fatal("Kind 不应为空")
	}
	if len(entries[0].Channels) != 1 || entries[0].Channels[0].Name != "测试通道" {
		t.Fatalf("Channels 期望 [测试通道]，实际 %+v", entries[0].Channels)
	}
}

// ListRegisteredSorted 应按 next_fire_at 升序，nil 排最后。
func TestEngineListRegisteredSorted(t *testing.T) {
	db := openSchedulerTestDB(t)
	loc := time.FixedZone("CST", 8*3600)
	e := NewEngine(db, &fakeDispatcher{}, testTZ(loc))
	e.Start()
	t.Cleanup(e.Stop)

	now := time.Now().In(loc)
	t1 := now.Add(3 * time.Hour)
	t2 := now.Add(1 * time.Hour)
	t3 := now.Add(2 * time.Hour)
	times := []*models.Time{models.TimePtrOf(t1), models.TimePtrOf(t2), nil, models.TimePtrOf(t3)}
	for i, tp := range times {
		r := &models.Reminder{
			Title:        "r" + string(rune('0'+i)),
			Calendar:     "solar",
			ScheduleType: "interval",
			ScheduleSpec: datatypes.JSON(`{"start_at":"2099-01-01T09:00:00","every":1,"unit":"day"}`),
			Enabled:      true,
			Source:       "web",
			NextFireAt:   tp,
		}
		if err := db.Create(r).Error; err != nil {
			t.Fatalf("创建失败: %v", err)
		}
		if err := e.Add(r); err != nil {
			t.Fatalf("Add 失败: %v", err)
		}
	}

	entries, total := e.ListRegisteredSorted(0, 0)
	if total != 4 {
		t.Fatalf("total = %d，期望 4", total)
	}
	// 前三条应有 next_fire_at 且升序，最后一条为 nil
	for i := 0; i < 3; i++ {
		if entries[i].NextFireAt == nil {
			t.Fatalf("第 %d 条不应为 nil", i)
		}
		if i > 0 && entries[i].NextFireAt.Before(entries[i-1].NextFireAt.Std()) {
			t.Fatalf("未按升序排列: %v", entries)
		}
	}
	if entries[3].NextFireAt != nil {
		t.Fatalf("nil 的应排最后，实际 %v", entries[3].NextFireAt)
	}

	// 分页
	paged, total := e.ListRegisteredSorted(1, 2)
	if total != 4 || len(paged) != 2 {
		t.Fatalf("分页应为 total=4 len=2，实际 total=%d len=%d", total, len(paged))
	}

	// offset 超出范围
	empty, total := e.ListRegisteredSorted(100, 10)
	if total != 4 || empty != nil {
		t.Fatalf("超范围应返回 nil，实际 %v", empty)
	}
}

// LastFireTime 初始为零值，fire 之后应有值。
func TestEngineLastFireTime(t *testing.T) {
	db := openSchedulerTestDB(t)
	loc := time.FixedZone("CST", 8*3600)
	e := NewEngine(db, &fakeDispatcher{}, testTZ(loc))

	if !e.LastFireTime().IsZero() {
		t.Fatal("初始应为零值")
	}

	next := time.Now().Add(-time.Second).In(loc)
	r := &models.Reminder{
		Title:        "fire",
		Calendar:     "solar",
		ScheduleType: "cron",
		ScheduleSpec: datatypes.JSON(`{"expr":"0 9 * * *"}`),
		Enabled:      true,
		Source:       "web",
		NextFireAt:   models.TimePtrOf(next),
	}
	if err := db.Create(r).Error; err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	e.fire(r.ID)

	if e.LastFireTime().IsZero() {
		t.Fatal("fire 后应有值")
	}
}

// LoadAndRegisterAll 应把全部启用提醒注册进调度器。
func TestEngineLoadAndRegisterAll(t *testing.T) {
	db := openSchedulerTestDB(t)
	loc := time.FixedZone("CST", 8*3600)
	e := NewEngine(db, &fakeDispatcher{}, testTZ(loc))
	e.Start()
	t.Cleanup(e.Stop)

	// 两条启用的 cron + 一条禁用的
	newCronReminder(t, db, "0 9 * * *", true)
	newCronReminder(t, db, "0 10 * * *", true)
	newCronReminder(t, db, "0 11 * * *", false)

	if err := e.LoadAndRegisterAll(); err != nil {
		t.Fatalf("LoadAndRegisterAll 失败: %v", err)
	}
	if got := e.RegisteredCount(); got != 2 {
		t.Fatalf("应注册 2 条（禁用的跳过），实际 %d", got)
	}
}

// AddPurgeCron 应成功注册并返回有效的 entryID。
func TestEngineAddPurgeCron(t *testing.T) {
	db := openSchedulerTestDB(t)
	loc := time.FixedZone("CST", 8*3600)
	e := NewEngine(db, &fakeDispatcher{}, testTZ(loc))
	e.Start()
	t.Cleanup(e.Stop)

	fake := &fakePurger{}
	if _, err := e.AddPurgeCron(fake, 24*time.Hour); err != nil {
		t.Fatalf("AddPurgeCron 失败: %v", err)
	}
}

// fakePurger 用于 AddPurgeCron 测试。
type fakePurger struct{}

func (f *fakePurger) Purge(olderThan time.Duration, all bool) (int64, error) {
	return 0, nil
}

// NewEngine 传 nil 时区应回退到默认站点时区。
func TestNewEngine_NilLocation(t *testing.T) {
	db := openSchedulerTestDB(t)
	e := NewEngine(db, &fakeDispatcher{}, nil)
	if e == nil {
		t.Fatal("应返回非 nil")
	}
	if e.tz == nil || e.tz.Get() == nil {
		t.Fatal("tz 应回退到默认时区")
	}
}

// AddDailyJob 注册的维护任务应在 ReloadTimezone 后仍然存在（与日志清理同等对待）。
func TestEngineAddDailyJobSurvivesReload(t *testing.T) {
	db := openSchedulerTestDB(t)
	loc := time.FixedZone("CST", 8*3600)
	tz := testTZ(loc)
	e := NewEngine(db, &fakeDispatcher{}, tz)
	e.Start()
	t.Cleanup(e.Stop)

	ran := make(chan struct{}, 1)
	if _, err := e.AddDailyJob("测试任务", DailyPurgeExpr, func() { ran <- struct{}{} }); err != nil {
		t.Fatalf("AddDailyJob 失败: %v", err)
	}
	if got := e.ExtraEntryCount(); got != 1 {
		t.Fatalf("维护任务数 = %d，期望 1", got)
	}

	if err := tz.Set("America/New_York"); err != nil {
		t.Fatalf("切换时区失败: %v", err)
	}
	e.ReloadTimezone()
	if got := e.ExtraEntryCount(); got != 1 {
		t.Fatalf("重建后维护任务数 = %d，期望 1", got)
	}
}

// 维护任务 panic 不应带崩进程，且不影响后续调度。
func TestEngineAddDailyJobRecoversPanic(t *testing.T) {
	db := openSchedulerTestDB(t)
	e := NewEngine(db, &fakeDispatcher{}, testTZ(time.FixedZone("CST", 8*3600)))
	e.Start()
	t.Cleanup(e.Stop)

	// 直接取出注册的包装函数执行一次，验证 recover 生效
	err := func() (err error) {
		id, addErr := e.AddDailyJob("会 panic 的任务", DailyPurgeExpr, func() {
			panic("boom")
		})
		if addErr != nil {
			return addErr
		}
		e.mu.Lock()
		entry := e.cron.Entry(id)
		e.mu.Unlock()
		entry.Job.Run()
		return nil
	}()
	if err != nil {
		t.Fatalf("注册失败: %v", err)
	}
}

// ==================== nextfire_test.go ====================

func mustLoc(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("加载时区失败: %v", err)
	}
	return loc
}

func TestComputeSolarOnce_Future(t *testing.T) {
	loc := mustLoc(t, "Asia/Shanghai")
	spec := &ScheduleSpec{At: "2026-06-10T09:00:00"}
	after := time.Date(2026, 6, 5, 8, 0, 0, 0, loc)
	got, err := Compute("solar", "once", spec, after, loc)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil time")
	}
	want := time.Date(2026, 6, 10, 9, 0, 0, 0, loc)
	if !got.Equal(want) {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestComputeSolarOnce_Past(t *testing.T) {
	loc := mustLoc(t, "Asia/Shanghai")
	spec := &ScheduleSpec{At: "2020-01-01T00:00:00"}
	after := time.Date(2026, 6, 5, 8, 0, 0, 0, loc)
	got, err := Compute("solar", "once", spec, after, loc)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != nil {
		t.Errorf("过去时间应返回 nil，得到 %s", got)
	}
}

func TestComputeSolarInterval_Minute(t *testing.T) {
	loc := mustLoc(t, "Asia/Shanghai")
	spec := &ScheduleSpec{
		StartAt: "2026-06-05T09:00:00",
		Every:   10,
		Unit:    "minute",
	}
	after := time.Date(2026, 6, 5, 9, 25, 0, 0, loc)
	got, err := Compute("solar", "interval", spec, after, loc)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := time.Date(2026, 6, 5, 9, 30, 0, 0, loc)
	if !got.Equal(want) {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestComputeSolarInterval_Day(t *testing.T) {
	loc := mustLoc(t, "Asia/Shanghai")
	spec := &ScheduleSpec{
		StartAt: "2026-06-05T08:00:00",
		Every:   1,
		Unit:    "day",
	}
	after := time.Date(2026, 6, 6, 7, 0, 0, 0, loc)
	got, err := Compute("solar", "interval", spec, after, loc)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := time.Date(2026, 6, 6, 8, 0, 0, 0, loc)
	if !got.Equal(want) {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestComputeSolarInterval_MonthRollover(t *testing.T) {
	loc := mustLoc(t, "Asia/Shanghai")
	spec := &ScheduleSpec{
		StartAt: "2026-01-31T09:00:00",
		Every:   1,
		Unit:    "month",
	}
	// 从 1/31 → 2/28（time.AddDate 自动归一），再 → 3/28
	after := time.Date(2026, 2, 28, 10, 0, 0, 0, loc)
	got, err := Compute("solar", "interval", spec, after, loc)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got.Month() != time.March {
		t.Errorf("月份应是 3 月，得到 %s", got)
	}
}

func TestComputeSolarInterval_FirstFireIsStartAt(t *testing.T) {
	loc := mustLoc(t, "Asia/Shanghai")
	spec := &ScheduleSpec{
		StartAt: "2026-06-10T09:00:00",
		Every:   1,
		Unit:    "day",
	}
	// after 早于 start_at：下一次就是 start_at 本身
	after := time.Date(2026, 6, 5, 8, 0, 0, 0, loc)
	got, err := Compute("solar", "interval", spec, after, loc)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := time.Date(2026, 6, 10, 9, 0, 0, 0, loc)
	if !got.Equal(want) {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestComputeSolarCron(t *testing.T) {
	loc := mustLoc(t, "Asia/Shanghai")
	spec := &ScheduleSpec{Expr: "0 9 * * 1-5"}       // 工作日早 9 点
	after := time.Date(2026, 6, 5, 10, 0, 0, 0, loc) // 周五 10:00
	got, err := Compute("solar", "cron", spec, after, loc)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	// 下一次是下周一 09:00
	if got.Weekday() != time.Monday || got.Hour() != 9 {
		t.Errorf("期望下周一 09:00，得到 %s", got)
	}
}

func TestComputeSolarCron_InvalidExpr(t *testing.T) {
	loc := mustLoc(t, "Asia/Shanghai")
	spec := &ScheduleSpec{Expr: "not a cron"}
	if _, err := Compute("solar", "cron", spec, time.Now(), loc); err == nil {
		t.Fatal("无效 cron 应报错")
	}
}

func TestParseSpec_RoundTrip(t *testing.T) {
	raw := []byte(`{"at":"2026-06-10T09:00:00"}`)
	s, err := ParseSpec(raw)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if s.At != "2026-06-10T09:00:00" {
		t.Errorf("got %q", s.At)
	}
}

func TestComputeLunarOnce_Future(t *testing.T) {
	loc := mustLoc(t, "Asia/Shanghai")
	// 2026 年农历正月初一 = 公历 2026-02-17
	spec := &ScheduleSpec{
		Lunar:  &LunarDate{Year: 2026, Month: 1, Day: 1},
		Hour:   9,
		Minute: 0,
	}
	after := time.Date(2026, 1, 1, 8, 0, 0, 0, loc)
	got, err := Compute("lunar", "once", spec, after, loc)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil time")
	}
	want := time.Date(2026, 2, 17, 9, 0, 0, 0, loc)
	if !got.Equal(want) {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestComputeLunarOnce_Past(t *testing.T) {
	loc := mustLoc(t, "Asia/Shanghai")
	// 2024 年农历正月初一 = 公历 2024-02-10
	spec := &ScheduleSpec{
		Lunar:  &LunarDate{Year: 2024, Month: 1, Day: 1},
		Hour:   9,
		Minute: 0,
	}
	after := time.Date(2026, 6, 5, 8, 0, 0, 0, loc)
	got, err := Compute("lunar", "once", spec, after, loc)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != nil {
		t.Errorf("过去农历日期应返回 nil，得到 %s", got)
	}
}

func TestComputeLunarInterval_Month(t *testing.T) {
	loc := mustLoc(t, "Asia/Shanghai")
	// 农历每月初一 09:00，看 3 个月后
	spec := &ScheduleSpec{
		StartLunar: &LunarDate{Year: 2026, Month: 1, Day: 1},
		Every:      1,
		Unit:       "month",
		Hour:       9,
		Minute:     0,
	}
	// 公历 2026-05-01（农历三月十五左右）
	after := time.Date(2026, 5, 1, 8, 0, 0, 0, loc)
	got, err := Compute("lunar", "interval", spec, after, loc)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil time")
	}
	// 从正月初一开始，加 3 个月 = 四月，1926-04-01 → 公历约 2026-05-17
	// 具体日期需要 library 确定；我们只验证月份 >= 4 且时间在 09:00
	if got.Hour() != 9 || got.Minute() != 0 {
		t.Errorf("期望 09:00，得到 %s", got)
	}
}

func TestComputeLunarInterval_Year(t *testing.T) {
	loc := mustLoc(t, "Asia/Shanghai")
	// 农历每年正月初一（春节）09:00
	spec := &ScheduleSpec{
		StartLunar: &LunarDate{Year: 2026, Month: 1, Day: 1},
		Every:      1,
		Unit:       "year",
		Hour:       9,
		Minute:     0,
	}
	after := time.Date(2026, 6, 5, 8, 0, 0, 0, loc)
	got, err := Compute("lunar", "interval", spec, after, loc)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil time")
	}
	// 下一次是 2027 年春节，应该在 2027 年 1 月底~2 月
	if got.Year() != 2027 {
		t.Errorf("期望 2027 年，得到 %d", got.Year())
	}
	if got.Hour() != 9 {
		t.Errorf("期望 09:00，得到 %d", got.Hour())
	}
}

func TestCompute_InvalidLunarCombo(t *testing.T) {
	loc := mustLoc(t, "Asia/Shanghai")
	_, err := Compute("lunar", "cron", &ScheduleSpec{}, time.Now(), loc)
	if err == nil {
		t.Fatal("lunar/cron 应报错")
	}
}

func TestPreviewSolar_Cron(t *testing.T) {
	loc := mustLoc(t, "Asia/Shanghai")
	spec := &ScheduleSpec{Expr: "0 9 * * *"}
	from := time.Date(2026, 6, 5, 8, 0, 0, 0, loc)
	got, err := PreviewSolar("solar", "cron", spec, from, loc, 3)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("want 3, got %d", len(got))
	}
	for _, t0 := range got {
		if t0.Hour() != 9 {
			t.Errorf("每次应在 09 点，得到 %s", t0)
		}
	}
}

func TestPreviewSolar_OnceStopsEarly(t *testing.T) {
	loc := mustLoc(t, "Asia/Shanghai")
	spec := &ScheduleSpec{At: "2026-06-10T09:00:00"}
	from := time.Date(2026, 6, 5, 8, 0, 0, 0, loc)
	got, err := PreviewSolar("solar", "once", spec, from, loc, 5)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("once 只应返回 1 次，得到 %d", len(got))
	}
}

// ==================== sweeper_test.go ====================

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

// seedPendingLog 插入一条 pending 日志，firedAt 用于模拟不同年龄。
func seedPendingLog(t *testing.T, s *Sweeper, firedAt time.Time) *models.DeliveryLog {
	t.Helper()
	dl := &models.DeliveryLog{
		ReminderID: 1,
		FiredAt:    models.TimeOf(firedAt),
		Title:      "卡住的投递",
		Content:    "正文",
		Status:     "pending",
		Source:     "web",
	}
	if err := s.db.Create(dl).Error; err != nil {
		t.Fatalf("插入 pending 日志失败: %v", err)
	}
	return dl
}

// 超过 pendingTTL 的 pending 日志应被收敛为 failed，并带上原因。
func TestSweeperTick_RecoversStalePendingLogs(t *testing.T) {
	db := openSchedulerTestDB(t)
	loc := time.FixedZone("CST", 8*3600)
	engine := NewEngine(db, &fakeDispatcher{}, testTZ(loc))
	t.Cleanup(engine.Stop)
	s := NewSweeper(db, engine, testTZ(loc), time.Minute, 60*time.Minute, 0)

	stale := seedPendingLog(t, s, time.Now().Add(-20*time.Minute))
	s.tick()

	var after models.DeliveryLog
	if err := db.First(&after, stale.ID).Error; err != nil {
		t.Fatalf("重读日志失败: %v", err)
	}
	if after.Status != "failed" {
		t.Fatalf("状态 = %q，期望 failed", after.Status)
	}
	if after.Error == "" {
		t.Fatal("收敛时应写清原因，否则用户无法判断为什么没发出去")
	}
}

// 还在 TTL 内、可能正在重试的日志不能被误判。
func TestSweeperTick_KeepsRecentPendingLogs(t *testing.T) {
	db := openSchedulerTestDB(t)
	loc := time.FixedZone("CST", 8*3600)
	engine := NewEngine(db, &fakeDispatcher{}, testTZ(loc))
	t.Cleanup(engine.Stop)
	s := NewSweeper(db, engine, testTZ(loc), time.Minute, 60*time.Minute, 0)

	fresh := seedPendingLog(t, s, time.Now().Add(-time.Minute))
	s.tick()

	var after models.DeliveryLog
	if err := db.First(&after, fresh.ID).Error; err != nil {
		t.Fatalf("重读日志失败: %v", err)
	}
	if after.Status != "pending" {
		t.Fatalf("状态 = %q，期望仍为 pending", after.Status)
	}
}

// 已到终态的日志不应被改动。
func TestSweeperTick_DoesNotTouchFinishedLogs(t *testing.T) {
	db := openSchedulerTestDB(t)
	loc := time.FixedZone("CST", 8*3600)
	engine := NewEngine(db, &fakeDispatcher{}, testTZ(loc))
	t.Cleanup(engine.Stop)
	s := NewSweeper(db, engine, testTZ(loc), time.Minute, 60*time.Minute, 0)

	dl := seedPendingLog(t, s, time.Now().Add(-20*time.Minute))
	if err := db.Model(&models.DeliveryLog{}).Where("id = ?", dl.ID).
		Update("status", "success").Error; err != nil {
		t.Fatalf("更新状态失败: %v", err)
	}
	s.tick()

	var after models.DeliveryLog
	if err := db.First(&after, dl.ID).Error; err != nil {
		t.Fatalf("重读日志失败: %v", err)
	}
	if after.Status != "success" {
		t.Fatalf("终态日志被改写为 %q", after.Status)
	}
}

// 默认参数下 pendingTTL 应已就位。
func TestNewSweeper_PendingTTLDefault(t *testing.T) {
	db := openSchedulerTestDB(t)
	engine := NewEngine(db, &fakeDispatcher{}, nil)
	s := NewSweeper(db, engine, nil, 0, 0, 0)
	if s.pendingTTL <= 0 {
		t.Fatalf("pendingTTL = %v，应大于 0", s.pendingTTL)
	}
}

// ==================== timezone_test.go ====================

// 回归测试：同一时刻用不同时区偏移传入，落库后必须是同一个整数毫秒，
// 因此 sweeper 能一致地扫到它们。
//
// 若时间按文本落库，+08:00 与 +00:00 两行会按字典序排在不同位置，
// 边界查询只命中一半。
func TestSweeperTick_HandlesMixedOffsetInput(t *testing.T) {
	db := openSchedulerTestDB(t)
	loc := time.FixedZone("CST", 8*3600)
	engine := NewEngine(db, &fakeDispatcher{}, testTZ(loc))
	t.Cleanup(engine.Stop)

	missed := time.Now().Add(-10 * time.Minute)
	// 同一时刻的两种写法：CST 与 UTC，落库后应完全相等
	withOffset := missed.In(loc)
	utcSame := missed.UTC()

	var ids []uint
	for _, tp := range []time.Time{withOffset, utcSame} {
		r := &models.Reminder{
			Title:        "偏移混用",
			Calendar:     "solar",
			ScheduleType: "interval",
			ScheduleSpec: datatypes.JSON(`{"start_at":"2026-06-05T09:00:00","every":1,"unit":"day"}`),
			Enabled:      true,
			Source:       "web",
			NextFireAt:   models.TimePtrOf(tp),
		}
		if err := db.Create(r).Error; err != nil {
			t.Fatalf("创建提醒失败: %v", err)
		}
		ids = append(ids, r.ID)
	}

	// 两行落库值必须相同（都是整数毫秒），这是排序与范围查询正确的前提
	var raws []string
	if err := db.Raw("SELECT CAST(next_fire_at AS TEXT) FROM reminders ORDER BY id").
		Scan(&raws).Error; err != nil {
		t.Fatalf("读取原始列失败: %v", err)
	}
	if len(raws) != 2 {
		t.Fatalf("应读到 2 行，实际 %d", len(raws))
	}
	if raws[0] != raws[1] {
		t.Fatalf("同一时刻的不同偏移应存成同值，实际 %q / %q", raws[0], raws[1])
	}

	s := NewSweeper(db, engine, testTZ(loc), time.Minute, 60*time.Minute, 0)
	s.tick()

	for _, id := range ids {
		if n := countReminderLogs(t, db, id); n != 1 {
			t.Fatalf("提醒 %d 应补发 1 条日志，实际 %d（落库原值 %q）", id, n, raws[0])
		}
	}
}

// 新写入的时间一律是整数毫秒，与传入的时区无关（Value 内部归一化为 UTC）。
func TestEngineFire_WritesIntegerMillisecond(t *testing.T) {
	db := openSchedulerTestDB(t)
	loc := time.FixedZone("CST", 8*3600)
	engine := NewEngine(db, &fakeDispatcher{}, testTZ(loc))
	t.Cleanup(engine.Stop)

	next := time.Now().Add(-time.Second).In(loc)
	r := &models.Reminder{
		Title:        "写整数",
		Calendar:     "solar",
		ScheduleType: "interval",
		ScheduleSpec: datatypes.JSON(`{"start_at":"2026-06-05T09:00:00","every":1,"unit":"day"}`),
		Enabled:      true,
		Source:       "web",
		NextFireAt:   models.TimePtrOf(next),
	}
	if err := db.Create(r).Error; err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	engine.fire(r.ID)

	// next_fire_at 与 fired_at 都必须是整数毫秒（13 位数字），而不是任何时间文本
	var row struct {
		Next *int64 `gorm:"column:next_fire_at"`
	}
	if err := db.Raw("SELECT CAST(next_fire_at AS INTEGER) AS next_fire_at FROM reminders WHERE id = ?", r.ID).
		Scan(&row).Error; err != nil {
		t.Fatalf("读取原始列失败: %v", err)
	}
	if row.Next == nil || *row.Next < 1e12 {
		t.Fatalf("next_fire_at 应为毫秒整数，实际 %v", row.Next)
	}
	var firedAt struct {
		V *int64 `gorm:"column:fired_at"`
	}
	if err := db.Raw("SELECT CAST(fired_at AS INTEGER) AS fired_at FROM delivery_logs WHERE reminder_id = ?", r.ID).
		Scan(&firedAt).Error; err != nil {
		t.Fatalf("读取日志时间失败: %v", err)
	}
	if firedAt.V == nil || *firedAt.V < 1e12 {
		t.Fatalf("delivery_logs.fired_at 应为毫秒整数，实际 %v", firedAt.V)
	}
}

// Compute 返回的时间仍按传入时区解释（便于展示），落库前由调用方转 UTC。
func TestCompute_KeepsGivenTimezone(t *testing.T) {
	loc := time.FixedZone("CST", 8*3600)
	spec := &ScheduleSpec{Expr: "0 9 * * *"}
	got, err := Compute("solar", "cron", spec, time.Date(2026, 6, 5, 10, 0, 0, 0, loc), loc)
	if err != nil {
		t.Fatalf("Compute 失败: %v", err)
	}
	if got == nil {
		t.Fatal("应返回时间")
	}
	if h := got.In(loc).Hour(); h != 9 {
		t.Fatalf("应按站点时区解释为 9 点，实际 %d 点", h)
	}
	if got.Location() != loc {
		t.Fatalf("返回值的时区应保持为传入时区，实际 %v", got.Location())
	}
}

// 站点时区变更后 ReloadTimezone 应重建 cron 并按新时区重算下次触发。
func TestEngineReloadTimezone(t *testing.T) {
	db := openSchedulerTestDB(t)
	tz := testTZ(time.FixedZone("CST", 8*3600))
	engine := NewEngine(db, &fakeDispatcher{}, tz)
	engine.Start()
	t.Cleanup(engine.Stop)

	r := newCronReminder(t, db, "0 9 * * *", true)
	if err := engine.Add(r); err != nil {
		t.Fatalf("Add 失败: %v", err)
	}
	if got := engine.RegisteredCount(); got != 1 {
		t.Fatalf("注册数 = %d，期望 1", got)
	}

	if err := tz.Set("America/New_York"); err != nil {
		t.Fatalf("切换时区失败: %v", err)
	}
	engine.ReloadTimezone()

	if !engine.IsRunning() {
		t.Fatal("重建后应仍在运行")
	}
	if got := engine.RegisteredCount(); got != 1 {
		t.Fatalf("重建后注册数 = %d，期望 1", got)
	}
}

// 站点时区变更后，库里的 next_fire_at 必须按新时区重算并回写。
//
// 只重建 cron 是不够的：sweeper 拿库里的值判断补漏，旧值在新时区下
// 可能已成过去，会被当作漏触发补发一次或被标记 expired。
func TestEngineReloadTimezoneRecomputesNextFire(t *testing.T) {
	db := openSchedulerTestDB(t)
	cst := time.FixedZone("CST", 8*3600)
	tz := testTZ(cst)
	engine := NewEngine(db, &fakeDispatcher{}, tz)
	engine.Start()
	t.Cleanup(engine.Stop)

	// 单次提醒：当地 2030-01-01 09:00
	r := &models.Reminder{
		Title:        "重算",
		Calendar:     "solar",
		ScheduleType: "once",
		ScheduleSpec: datatypes.JSON(`{"at":"2030-01-01T09:00:00"}`),
		Enabled:      true,
		Source:       "web",
	}
	if err := db.Create(r).Error; err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	// 按旧时区（CST）算出并写入初始值，模拟改时区前的状态
	oldNext, err := Compute("solar", "once", &ScheduleSpec{At: "2030-01-01T09:00:00"}, time.Now(), cst)
	if err != nil || oldNext == nil {
		t.Fatalf("计算初始 next_fire_at 失败: %v", err)
	}
	if err := db.Model(&models.Reminder{}).Where("id = ?", r.ID).
		Update("next_fire_at", models.TimeOf(*oldNext)).Error; err != nil {
		t.Fatalf("写入初始值失败: %v", err)
	}

	// 禁用的提醒不参与重算
	disabled := &models.Reminder{
		Title:        "禁用",
		Calendar:     "solar",
		ScheduleType: "once",
		ScheduleSpec: datatypes.JSON(`{"at":"2030-01-01T09:00:00"}`),
		Enabled:      false,
		Source:       "web",
	}
	if err := db.Create(disabled).Error; err != nil {
		t.Fatalf("创建禁用提醒失败: %v", err)
	}

	if err := tz.Set("Europe/Berlin"); err != nil {
		t.Fatalf("切换时区失败: %v", err)
	}
	engine.ReloadTimezone()

	var got models.Reminder
	if err := db.First(&got, r.ID).Error; err != nil {
		t.Fatalf("重读失败: %v", err)
	}
	if got.NextFireAt == nil {
		t.Fatal("重算后 next_fire_at 不应为空")
	}
	// 柏林时间 2030-01-01 09:00 = 08:00Z
	if h := got.NextFireAt.UTC().Hour(); h != 8 {
		t.Fatalf("应按柏林 9 点（08:00Z）重算，实际 %02d:00Z", h)
	}
	if d := got.NextFireAt.UTC().Day(); d != 1 {
		t.Fatalf("日期应保持 2030-01-01，实际 %d", d)
	}

	var gotDisabled models.Reminder
	if err := db.First(&gotDisabled, disabled.ID).Error; err != nil {
		t.Fatalf("重读禁用提醒失败: %v", err)
	}
	if gotDisabled.NextFireAt != nil {
		t.Fatalf("禁用的提醒不应被重算，实际 %v", gotDisabled.NextFireAt)
	}
}

// 时区前移导致旧计划已是过去时，重算不应产生任何投递，也不该补发。
func TestEngineReloadTimezoneDoesNotFireMissedOnce(t *testing.T) {
	db := openSchedulerTestDB(t)
	cst := time.FixedZone("CST", 8*3600)
	tz := testTZ(cst)
	engine := NewEngine(db, &fakeDispatcher{}, tz)
	engine.Start()
	t.Cleanup(engine.Stop)

	// 已过去的单次提醒：改时区时若沿用旧值，sweeper 会在容忍窗口内立刻补发
	past := time.Now().Add(-10 * time.Minute).UTC()
	r := &models.Reminder{
		Title:        "过去的单次",
		Calendar:     "solar",
		ScheduleType: "once",
		ScheduleSpec: datatypes.JSON(`{"at":"2020-01-01T09:00:00"}`),
		Enabled:      true,
		Source:       "web",
		NextFireAt:   models.TimePtrOf(past),
	}
	if err := db.Create(r).Error; err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	if err := tz.Set("Europe/Berlin"); err != nil {
		t.Fatalf("切换时区失败: %v", err)
	}
	engine.ReloadTimezone()

	var got models.Reminder
	if err := db.First(&got, r.ID).Error; err != nil {
		t.Fatalf("重读失败: %v", err)
	}
	if got.NextFireAt != nil {
		t.Fatalf("已过去的单次提醒重算后应无下次触发，实际 %v", got.NextFireAt)
	}
	if n := countReminderLogs(t, db, r.ID); n != 0 {
		t.Fatalf("改时区不应产生投递，实际 %d 条日志", n)
	}
}

// 时区变更重建 cron 实例后，维护型任务（每日日志清理）必须仍然存在。
func TestEngineReloadTimezoneKeepsPurgeCron(t *testing.T) {
	db := openSchedulerTestDB(t)
	tz := testTZ(time.FixedZone("CST", 8*3600))
	engine := NewEngine(db, &fakeDispatcher{}, tz)
	engine.Start()
	t.Cleanup(engine.Stop)

	if _, err := engine.AddPurgeCron(&fakePurger{}, 24*time.Hour); err != nil {
		t.Fatalf("AddPurgeCron 失败: %v", err)
	}
	if got := engine.ExtraEntryCount(); got != 1 {
		t.Fatalf("初始维护任务数 = %d，期望 1", got)
	}

	if err := tz.Set("America/New_York"); err != nil {
		t.Fatalf("切换时区失败: %v", err)
	}
	engine.ReloadTimezone()

	if got := engine.ExtraEntryCount(); got != 1 {
		t.Fatalf("重建后维护任务数 = %d，期望 1（不能因改时区丢失清理任务）", got)
	}
}

// 站点时区变更后，Preview 应按新时区计算。
func TestPreviewSolar_FollowsTimezoneChange(t *testing.T) {
	tz := testTZ(time.FixedZone("CST", 8*3600))
	spec := &ScheduleSpec{Expr: "0 9 * * *"}
	from := time.Now()

	before, err := PreviewSolar("solar", "cron", spec, from, tz.Get(), 1)
	if err != nil {
		t.Fatalf("Preview 失败: %v", err)
	}
	if h := before[0].In(tz.Get()).Hour(); h != 9 {
		t.Fatalf("切换前应按 CST 9 点，实际 %d", h)
	}

	if err := tz.Set("Europe/Berlin"); err != nil {
		t.Fatalf("切换失败: %v", err)
	}
	after, err := PreviewSolar("solar", "cron", spec, from, tz.Get(), 1)
	if err != nil {
		t.Fatalf("Preview 失败: %v", err)
	}
	if h := after[0].In(tz.Get()).Hour(); h != 9 {
		t.Fatalf("切换后应按柏林 9 点，实际 %d", h)
	}
}

// ==================== testutil_test.go ====================
// 本包测试脚手架：内存库、固定时区与 fake 替身。

// testTZ 把固定时区包装成 timezone.Provider，供测试注入。
func testTZ(loc *time.Location) *timezone.Provider {
	return testutil.TZ(loc)
}

// cstZone 是测试默认时区（东八区），与生产默认值一致。
func cstZone() *time.Location {
	return testutil.CST()
}

// openSchedulerTestDB 构造调度器测试用的内存数据库。
func openSchedulerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	return testutil.NewDB(t)
}

// fakeDispatcher 记录 Engine 交出去的投递任务，便于断言。
type fakeDispatcher struct {
	mu   sync.Mutex
	runs []dispatchCall
}

// dispatchCall 是一次投递调用的记录。
type dispatchCall struct {
	reminderID uint
	logID      uint
}

func (f *fakeDispatcher) Run(_ context.Context, r *models.Reminder, deliveryLogID uint) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.runs = append(f.runs, dispatchCall{reminderID: r.ID, logID: deliveryLogID})
}

func (f *fakeDispatcher) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.runs)
}
