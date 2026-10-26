package services

import (
	"testing"
	"time"

	"github.com/reminder/backend/internal/models"
	"gorm.io/gorm"
)

// seedReminderForUpcoming 插入一条带 next_fire_at 的启用提醒。
func seedReminderForUpcoming(t *testing.T, db *gorm.DB, title string, next time.Time, enabled bool) *models.Reminder {
	t.Helper()
	r := &models.Reminder{
		Title:        title,
		Calendar:     "solar",
		ScheduleType: "once",
		Enabled:      true,
		Source:       "web",
		NextFireAt:   &next,
	}
	if err := db.Create(r).Error; err != nil {
		t.Fatalf("创建提醒失败: %v", err)
	}
	if !enabled {
		if err := db.Model(&models.Reminder{}).Where("id = ?", r.ID).
			Update("enabled", false).Error; err != nil {
			t.Fatalf("禁用提醒失败: %v", err)
		}
	}
	return r
}

// Upcoming 只返回窗口内、已启用、有 next_fire_at 的提醒，按时间升序。
func TestReminderUpcoming(t *testing.T) {
	db := openTestDB(t)
	svc := newReminderStack(t, db)

	now := time.Now()
	seedReminderForUpcoming(t, db, "1小时后", now.Add(time.Hour), true)
	seedReminderForUpcoming(t, db, "10小时后", now.Add(10*time.Hour), true)
	seedReminderForUpcoming(t, db, "2天后", now.Add(48*time.Hour), true) // 超出 24h 窗口
	seedReminderForUpcoming(t, db, "已禁用", now.Add(2*time.Hour), false)

	items, err := svc.Upcoming(24*time.Hour, 10)
	if err != nil {
		t.Fatalf("Upcoming 失败: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("应返回 2 条，实际 %d", len(items))
	}
	if items[0].Title != "1小时后" {
		t.Fatalf("应按时间升序，首条 = %q", items[0].Title)
	}
	if items[1].Title != "10小时后" {
		t.Fatalf("第二条 = %q", items[1].Title)
	}
}

// Upcoming 的 limit：<=0 时默认 10，超过 50 时截断为 50。
func TestReminderUpcoming_LimitClamped(t *testing.T) {
	db := openTestDB(t)
	svc := newReminderStack(t, db)

	now := time.Now()
	for i := 0; i < 3; i++ {
		seedReminderForUpcoming(t, db, "r", now.Add(time.Duration(i+1)*time.Minute), true)
	}

	// limit=0 → 默认 10，应返回全部 3 条
	items, err := svc.Upcoming(time.Hour, 0)
	if err != nil {
		t.Fatalf("Upcoming 失败: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("应返回 3 条，实际 %d", len(items))
	}
}

// UpcomingBetween 返回指定区间内的提醒，limit=0 表示不限。
func TestReminderUpcomingBetween(t *testing.T) {
	db := openTestDB(t)
	svc := newReminderStack(t, db)

	base := time.Date(2030, 6, 15, 12, 0, 0, 0, time.UTC)
	seedReminderForUpcoming(t, db, "区间内", base, true)
	seedReminderForUpcoming(t, db, "区间外", base.Add(72*time.Hour), true)

	from := base.Add(-time.Hour)
	to := base.Add(time.Hour)
	items, err := svc.UpcomingBetween(from, to, 0)
	if err != nil {
		t.Fatalf("UpcomingBetween 失败: %v", err)
	}
	if len(items) != 1 || items[0].Title != "区间内" {
		t.Fatalf("应只返回区间内的 1 条，实际 %v", items)
	}
}

// Preview 计算未来 n 次触发；n 会被夹在 [1, 20]。
func TestReminderPreview(t *testing.T) {
	db := openTestDB(t)
	svc := newReminderStack(t, db)

	spec := map[string]any{"expr": "0 9 * * *"}
	times, err := svc.Preview("solar", "cron", spec, "", 3)
	if err != nil {
		t.Fatalf("Preview 失败: %v", err)
	}
	if len(times) != 3 {
		t.Fatalf("应返回 3 个时间，实际 %d", len(times))
	}

	// n=0 走默认 5
	times, err = svc.Preview("solar", "cron", spec, "", 0)
	if err != nil {
		t.Fatalf("Preview 失败: %v", err)
	}
	if len(times) != 5 {
		t.Fatalf("n=0 应默认 5 个，实际 %d", len(times))
	}

	// n 超过 20 应截断
	times, err = svc.Preview("solar", "cron", spec, "", 100)
	if err != nil {
		t.Fatalf("Preview 失败: %v", err)
	}
	if len(times) != 20 {
		t.Fatalf("n=100 应截断为 20，实际 %d", len(times))
	}

	// 非法 spec 应报错
	if _, err := svc.Preview("solar", "cron", map[string]any{"expr": "not a cron"}, "", 3); err == nil {
		t.Fatal("非法 cron 应报错")
	}
}

// Preview 使用指定时区计算。
func TestReminderPreview_Timezone(t *testing.T) {
	db := openTestDB(t)
	svc := newReminderStack(t, db)

	times, err := svc.Preview("solar", "cron", map[string]any{"expr": "0 9 * * *"}, "America/New_York", 2)
	if err != nil {
		t.Fatalf("Preview 失败: %v", err)
	}
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("加载时区失败: %v", err)
	}
	for _, tm := range times {
		if h := tm.In(loc).Hour(); h != 9 {
			t.Fatalf("应按纽约时区 9 点触发，实际 %d 点 (%s)", h, tm)
		}
	}
}

// TestDryRun：无通道、空标题、通道不存在、正常发送。
func TestReminderTestDryRun(t *testing.T) {
	db := openTestDB(t)
	svc := newReminderStack(t, db)

	if err := svc.TestDryRun(t.Context(), "t", "c", "text", nil); err == nil {
		t.Fatal("无通道应报错")
	}
	if err := svc.TestDryRun(t.Context(), "  ", "c", "text", []uint{1}); err == nil {
		t.Fatal("空标题应报错")
	}
	if err := svc.TestDryRun(t.Context(), "t", "c", "text", []uint{9999}); err == nil {
		t.Fatal("通道不存在应报错")
	}

	ch := createChannel(t, db, "log", true)
	if err := svc.TestDryRun(t.Context(), "试发", "正文 {{now_date}}", "text", []uint{ch.ID}); err != nil {
		t.Fatalf("正常试发应成功: %v", err)
	}
}

// loadChannelsByIDs 只加载 enabled 的通道。
func TestLoadChannelsByIDs(t *testing.T) {
	db := openTestDB(t)
	svc := newReminderStack(t, db)

	enabled := createChannel(t, db, "log", true)
	disabled := createChannel(t, db, "log", false)

	got, err := svc.loadChannelsByIDs([]uint{enabled.ID, disabled.ID})
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}
	if len(got) != 1 || got[0].ID != enabled.ID {
		t.Fatalf("应只加载 enabled 的 1 个通道，实际 %d 个", len(got))
	}

	// 空输入返回 nil
	got, err = svc.loadChannelsByIDs(nil)
	if err != nil || got != nil {
		t.Fatalf("空输入应返回 nil, nil，实际 %v, %v", got, err)
	}
}
