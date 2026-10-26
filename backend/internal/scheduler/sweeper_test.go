package scheduler

import (
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
	engine := NewEngine(db, disp, loc)
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
		NextFireAt:   &missed,
	}
	if err := db.Create(r).Error; err != nil {
		t.Fatalf("创建提醒失败: %v", err)
	}

	s := NewSweeper(db, engine, loc, time.Minute, 60*time.Minute, 0)
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
	engine := NewEngine(db, &fakeDispatcher{}, loc)
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
		NextFireAt:   &missed,
	}
	if err := db.Create(r).Error; err != nil {
		t.Fatalf("创建提醒失败: %v", err)
	}

	s := NewSweeper(db, engine, loc, time.Minute, 60*time.Minute, 0)
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
	engine := NewEngine(db, &fakeDispatcher{}, loc)
	t.Cleanup(engine.Stop)

	missed := time.Now().Add(-48 * time.Hour).In(loc)
	r := &models.Reminder{
		Title:        "一次性过期",
		Calendar:     "solar",
		ScheduleType: "once",
		ScheduleSpec: datatypes.JSON(`{"at":"2020-01-01T09:00:00"}`),
		Enabled:      true,
		Source:       "web",
		NextFireAt:   &missed,
	}
	if err := db.Create(r).Error; err != nil {
		t.Fatalf("创建提醒失败: %v", err)
	}

	s := NewSweeper(db, engine, loc, time.Minute, 60*time.Minute, 0)
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
	engine := NewEngine(db, &fakeDispatcher{}, loc)
	t.Cleanup(engine.Stop)

	future := time.Now().Add(time.Hour).In(loc)
	r := &models.Reminder{
		Title:        "未到期",
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

	s := NewSweeper(db, engine, loc, time.Minute, 60*time.Minute, 0)
	s.tick()

	if n := countReminderLogs(t, db, r.ID); n != 0 {
		t.Fatalf("未到期提醒不应产生日志，实际 %d 条", n)
	}
}
