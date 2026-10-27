package scheduler

import (
	"testing"
	"time"

	"github.com/reminder/backend/internal/models"
	"gorm.io/datatypes"
)

// 回归测试：升级前写入的行是带本地偏移的文本（如 +08:00），
// 而查询边界是 UTC。若按文本比较，两条时间相同的记录会被排成不同位置，
// sweeper 会一条也扫不到（这正是 CI 上 TestSweeperTick_* 失败的根因）。
func TestSweeperTick_HandlesLegacyLocalOffsetRows(t *testing.T) {
	db := openSchedulerTestDB(t)
	loc := time.FixedZone("CST", 8*3600)
	engine := NewEngine(db, &fakeDispatcher{}, testTZ(loc))
	t.Cleanup(engine.Stop)

	missed := time.Now().Add(-10 * time.Minute)
	// 同一时刻的两种存法：历史数据是 +08:00，新数据是 +00:00
	legacy := missed.In(loc)
	modern := missed.UTC()

	var ids []uint
	for _, tp := range []*time.Time{&legacy, &modern} {
		r := &models.Reminder{
			Title:        "偏移兼容",
			Calendar:     "solar",
			ScheduleType: "interval",
			ScheduleSpec: datatypes.JSON(`{"start_at":"2026-06-05T09:00:00","every":1,"unit":"day"}`),
			Enabled:      true,
			Source:       "web",
			NextFireAt:   tp,
		}
		if err := db.Create(r).Error; err != nil {
			t.Fatalf("创建提醒失败: %v", err)
		}
		ids = append(ids, r.ID)
	}

	// 两种写法分别落库：legacy 会存成 "...+08:00"，modern 存成 "...+00:00"。
	// 这正是升级前后的真实数据形态。
	var raw string
	if err := db.Raw("SELECT CAST(next_fire_at AS TEXT) FROM reminders WHERE id = ?", ids[0]).
		Scan(&raw).Error; err != nil {
		t.Fatalf("读取原始列失败: %v", err)
	}
	if got := raw[len(raw)-6:]; got != "+08:00" {
		t.Fatalf("历史数据应为 +08:00 偏移，实际 %q", raw)
	}

	s := NewSweeper(db, engine, testTZ(loc), time.Minute, 60*time.Minute, 0)
	s.tick()

	for _, id := range ids {
		if n := countReminderLogs(t, db, id); n != 1 {
			t.Fatalf("提醒 %d 应补发 1 条日志，实际 %d（落库原值 %q）", id, n, raw)
		}
	}
}

// 新写入的时间一律为 UTC，保证文本序等于时间序（索引与排序才有意义）。
func TestEngineFire_WritesUTCNextFireAt(t *testing.T) {
	db := openSchedulerTestDB(t)
	loc := time.FixedZone("CST", 8*3600)
	engine := NewEngine(db, &fakeDispatcher{}, testTZ(loc))
	t.Cleanup(engine.Stop)

	next := time.Now().Add(-time.Second).In(loc)
	r := &models.Reminder{
		Title:        "写 UTC",
		Calendar:     "solar",
		ScheduleType: "interval",
		ScheduleSpec: datatypes.JSON(`{"start_at":"2026-06-05T09:00:00","every":1,"unit":"day"}`),
		Enabled:      true,
		Source:       "web",
		NextFireAt:   &next,
	}
	if err := db.Create(r).Error; err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	engine.fire(r.ID)

	var text string
	if err := db.Raw("SELECT CAST(next_fire_at AS TEXT) FROM reminders WHERE id = ?", r.ID).
		Scan(&text).Error; err != nil {
		t.Fatalf("读取原始列失败: %v", err)
	}
	if got := text[len(text)-6:]; got != "+00:00" {
		t.Fatalf("next_fire_at 应以 UTC 落库，实际文本 %q", text)
	}

	// 日志的 fired_at 同样应为 UTC
	var logText string
	if err := db.Raw("SELECT CAST(fired_at AS TEXT) FROM delivery_logs WHERE reminder_id = ?", r.ID).
		Scan(&logText).Error; err != nil {
		t.Fatalf("读取日志时间失败: %v", err)
	}
	if got := logText[len(logText)-6:]; got != "+00:00" {
		t.Fatalf("delivery_logs.fired_at 应以 UTC 落库，实际文本 %q", logText)
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
