package scheduler

import (
	"testing"
	"time"

	"github.com/reminder/backend/internal/models"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

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
	e := NewEngine(db, &fakeDispatcher{}, loc)

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
	e := NewEngine(db, &fakeDispatcher{}, loc)
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
	e := NewEngine(db, &fakeDispatcher{}, loc)
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
	e := NewEngine(db, &fakeDispatcher{}, loc)
	t.Cleanup(e.Stop)

	if err := e.Add(nil); err == nil {
		t.Fatal("nil reminder 应报错")
	}
}

// 一次性提醒已过期时不注册（Compute 返回 nil）。
func TestEngineAddAfterFunc_ExpiredOnce(t *testing.T) {
	db := openSchedulerTestDB(t)
	loc := time.FixedZone("CST", 8*3600)
	e := NewEngine(db, &fakeDispatcher{}, loc)
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
	e := NewEngine(db, &fakeDispatcher{}, loc)
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
		NextFireAt:   &future,
	}
	if err := db.Create(r).Error; err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	if err := e.Add(r); err != nil {
		t.Fatalf("Add 失败: %v", err)
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
}

// ListRegisteredSorted 应按 next_fire_at 升序，nil 排最后。
func TestEngineListRegisteredSorted(t *testing.T) {
	db := openSchedulerTestDB(t)
	loc := time.FixedZone("CST", 8*3600)
	e := NewEngine(db, &fakeDispatcher{}, loc)
	e.Start()
	t.Cleanup(e.Stop)

	now := time.Now().In(loc)
	t1 := now.Add(3 * time.Hour)
	t2 := now.Add(1 * time.Hour)
	t3 := now.Add(2 * time.Hour)
	times := []*time.Time{&t1, &t2, nil, &t3}
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
		if i > 0 && entries[i].NextFireAt.Before(*entries[i-1].NextFireAt) {
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
	e := NewEngine(db, &fakeDispatcher{}, loc)

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
		NextFireAt:   &next,
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
	e := NewEngine(db, &fakeDispatcher{}, loc)
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
	e := NewEngine(db, &fakeDispatcher{}, loc)
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

// NewEngine 传 nil 时区应回退到 Local。
func TestNewEngine_NilLocation(t *testing.T) {
	db := openSchedulerTestDB(t)
	e := NewEngine(db, &fakeDispatcher{}, nil)
	if e == nil {
		t.Fatal("应返回非 nil")
	}
	if e.loc == nil {
		t.Fatal("loc 应回退到 Local")
	}
}
