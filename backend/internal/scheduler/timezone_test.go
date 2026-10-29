package scheduler

import (
	"testing"
	"time"

	"github.com/reminder/backend/internal/models"
	"gorm.io/datatypes"
)

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
