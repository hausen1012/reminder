package database

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/reminder/backend/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// 旧版本的建表 DDL：时间列是 datetime（文本），并带 next_fire_at 单列索引。
// 与改造前 AutoMigrate 生成的结构一致。
const legacyRemindersDDL = `
CREATE TABLE reminders (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    title TEXT NOT NULL,
    content TEXT,
    content_format TEXT DEFAULT "text",
    calendar TEXT,
    schedule_type TEXT,
    schedule_spec JSON,
    enabled numeric,
    source TEXT,
    token_id INTEGER,
    require_confirm numeric DEFAULT false,
    confirm_retry_interval_sec INTEGER DEFAULT 0,
    confirm_max_retries INTEGER DEFAULT 0,
    next_fire_at datetime,
    last_fired_at datetime,
    fire_count INTEGER,
    created_at datetime,
    updated_at datetime
);
CREATE INDEX idx_reminders_calendar ON reminders(calendar);
CREATE INDEX idx_reminders_enabled ON reminders(enabled);
CREATE INDEX idx_reminders_next_fire_at ON reminders(next_fire_at);
`

// 完整的升级路径：旧库（文本时间 + 旧索引）→ AutoMigrate → 归一化，
// 之后数据必须可查、可排序，且查询走索引。
func TestUpgradeFromLegacySchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "upgrade.db")
	db, err := gorm.Open(sqlite.Open(path), gormConfig())
	if err != nil {
		t.Fatalf("打开数据库失败: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("获取 sql.DB 失败: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	if err := db.Exec(legacyRemindersDDL).Error; err != nil {
		t.Fatalf("建旧表失败: %v", err)
	}

	// 旧数据：+00:00、+08:00、NULL 三种形态，其中前两条是同一时刻
	base := time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)
	rows := []struct {
		title string
		next  any
	}{
		{"UTC 形态", base.Format("2006-01-02 15:04:05.999999999-07:00")},
		{"CST 形态", base.In(time.FixedZone("CST", 8*3600)).Format("2006-01-02 15:04:05.999999999-07:00")},
		{"无时间", nil},
		{"更早", base.Add(-time.Hour).Format("2006-01-02 15:04:05.999999999-07:00")},
	}
	for _, r := range rows {
		if err := db.Exec(`INSERT INTO reminders (title, content_format, calendar, schedule_type, enabled, source, next_fire_at, created_at, updated_at, fire_count)
			VALUES (?, 'text', 'solar', 'once', 1, 'web', ?, ?, ?, 0)`,
			r.title, r.next, base.Format("2006-01-02 15:04:05-07:00"), base.Format("2006-01-02 15:04:05-07:00")).Error; err != nil {
			t.Fatalf("插入旧数据失败: %v", err)
		}
	}

	// 走一遍真实的升级流程
	if err := db.AutoMigrate(&models.Reminder{}); err != nil {
		t.Fatalf("AutoMigrate 失败: %v", err)
	}
	if _, err := normalizeTimeColumns(db); err != nil {
		t.Fatalf("归一化失败: %v", err)
	}

	// 1) 所有时间列都应是 integer 或 null，不能残留 text
	var leftovers []struct {
		Col string `gorm:"column:col"`
		Cnt int64  `gorm:"column:cnt"`
	}
	if err := db.Raw(`SELECT 'next_fire_at' AS col, COUNT(*) AS cnt FROM reminders WHERE typeof(next_fire_at) = 'text'
		UNION ALL SELECT 'last_fired_at', COUNT(*) FROM reminders WHERE typeof(last_fired_at) = 'text'
		UNION ALL SELECT 'created_at', COUNT(*) FROM reminders WHERE typeof(created_at) = 'text'
		UNION ALL SELECT 'updated_at', COUNT(*) FROM reminders WHERE typeof(updated_at) = 'text'`).
		Scan(&leftovers).Error; err != nil {
		t.Fatalf("检查列类型失败: %v", err)
	}
	for _, l := range leftovers {
		if l.Cnt != 0 {
			t.Fatalf("列 %s 仍有 %d 行是文本", l.Col, l.Cnt)
		}
	}

	// 2) 同一时刻的两条应相等；读回的时间也应是正确的时刻
	var all []models.Reminder
	if err := db.Order("next_fire_at ASC").Find(&all).Error; err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	byTitle := map[string]*models.Reminder{}
	for i := range all {
		byTitle[all[i].Title] = &all[i]
	}
	utcRow, cstRow := byTitle["UTC 形态"], byTitle["CST 形态"]
	if utcRow == nil || cstRow == nil {
		t.Fatalf("数据丢失，实际有 %d 行", len(all))
	}
	if !utcRow.NextFireAt.Equal(cstRow.NextFireAt.Std()) {
		t.Fatalf("同一时刻的两种偏移转换后应相等：%v vs %v", utcRow.NextFireAt.Time, cstRow.NextFireAt.Time)
	}
	if !utcRow.NextFireAt.Equal(base) {
		t.Fatalf("时间值转换错误：%v，期望 %v", utcRow.NextFireAt.Time, base)
	}
	if byTitle["无时间"].NextFireAt != nil {
		t.Fatalf("NULL 应保持 NULL，实际 %v", byTitle["无时间"].NextFireAt)
	}

	// 3) 排序：SQLite 的 NULL 最小，所以先跳过空值再看时间序
	var nonNull []models.Reminder
	if err := db.Where("next_fire_at IS NOT NULL").Order("next_fire_at ASC").Find(&nonNull).Error; err != nil {
		t.Fatalf("排序查询失败: %v", err)
	}
	if len(nonNull) != 3 {
		t.Fatalf("应有 3 条非空时间，实际 %d", len(nonNull))
	}
	if nonNull[0].Title != "更早" {
		t.Fatalf("排序错误，首行 = %q", nonNull[0].Title)
	}
	if !nonNull[1].NextFireAt.Equal(nonNull[2].NextFireAt.Std()) {
		t.Fatalf("两条同刻应相邻且相等：%v / %v", nonNull[1].NextFireAt.Time, nonNull[2].NextFireAt.Time)
	}

	// 4) 典型调度查询必须用上索引
	var plan []struct {
		Detail string `gorm:"column:detail"`
	}
	if err := db.Raw(`EXPLAIN QUERY PLAN
		SELECT * FROM reminders
		WHERE enabled = ? AND next_fire_at IS NOT NULL AND next_fire_at <= ?
		ORDER BY next_fire_at ASC LIMIT 10`, true, models.TimeOf(base)).Scan(&plan).Error; err != nil {
		t.Fatalf("执行计划查询失败: %v", err)
	}
	usedIndex := false
	for _, p := range plan {
		t.Logf("计划: %s", p.Detail)
		if len(p.Detail) > 6 && p.Detail[:6] == "SEARCH" {
			usedIndex = true
		}
	}
	if !usedIndex {
		t.Fatal("升级后查询未用上索引")
	}

	// 5) 升级后新写入的数据与历史数据能一起被查到
	if err := db.Create(&models.Reminder{
		Title: "升级后新增", Calendar: "solar", ScheduleType: "once", Enabled: true,
		NextFireAt: models.TimePtrOf(base.Add(-2 * time.Hour)),
	}).Error; err != nil {
		t.Fatalf("新增失败: %v", err)
	}
	var n int64
	if err := db.Model(&models.Reminder{}).
		Where("enabled = ? AND next_fire_at IS NOT NULL AND next_fire_at <= ?", true, models.TimeOf(base)).
		Count(&n).Error; err != nil {
		t.Fatalf("计数失败: %v", err)
	}
	// 三条旧的（UTC/CST/更早）+ 一条新的
	if n != 4 {
		t.Fatalf("应命中 4 行，实际 %d", n)
	}
}
