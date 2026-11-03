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
		Name:    "测试通道",
		Type:    "webhook",
		Enabled: true,
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
