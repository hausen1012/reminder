package database

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/reminder/backend/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// openLegacyDB 建一个"旧版本形态"的库：时间列是段文本，值与改造前一致。
func openLegacyDB(t *testing.T, schema func(*gorm.DB)) *gorm.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, err := gorm.Open(sqlite.Open(path), gormConfig())
	if err != nil {
		t.Fatalf("打开数据库失败: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("获取 sql.DB 失败: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	schema(db)
	return db
}

// 旧库里的文本时间必须被转成整数毫秒，且转换后按整数语义比较是正确的。
func TestNormalizeTimeColumns_ConvertsLegacyText(t *testing.T) {
	db := openLegacyDB(t, func(db *gorm.DB) {
		if err := db.Exec(`CREATE TABLE reminders (
			id INTEGER PRIMARY KEY,
			title TEXT,
			next_fire_at datetime,
			created_at datetime
		)`).Error; err != nil {
			t.Fatalf("建旧表失败: %v", err)
		}
		// 三种历史形态：+00:00、+08:00、空值
		rows := []struct {
			id   int
			next any
		}{
			{1, "2026-06-26 12:00:00.123456789+00:00"},
			{2, "2026-06-26 20:00:00+08:00"}, // 同一时刻，偏移不同
			{3, nil},
		}
		for _, r := range rows {
			if err := db.Exec(`INSERT INTO reminders (id, title, next_fire_at, created_at) VALUES (?, ?, ?, ?)`,
				r.id, "旧提醒", r.next, "2026-06-26 12:00:00+00:00").Error; err != nil {
				t.Fatalf("插入旧数据失败: %v", err)
			}
		}
	})

	n, err := normalizeTimeColumns(db)
	if err != nil {
		t.Fatalf("归一化失败: %v", err)
	}
	if n == 0 {
		t.Fatal("应转换到若干行，实际 0")
	}

	// 转换后：两行同一时刻应相等，NULL 仍为 NULL，且值应为整数毫秒
	type row struct {
		ID   int    `gorm:"column:id"`
		Type string `gorm:"column:t"`
		Text string `gorm:"column:txt"`
	}
	var rows []row
	if err := db.Raw(`SELECT id, typeof(next_fire_at) AS t, CAST(next_fire_at AS TEXT) AS txt FROM reminders ORDER BY id`).
		Scan(&rows).Error; err != nil {
		t.Fatalf("读取结果失败: %v", err)
	}
	wantText := map[int]string{
		1: "1782475200123",
		2: "1782475200000",
		3: "",
	}
	for _, r := range rows {
		if r.Text != wantText[r.ID] {
			t.Fatalf("id=%d 转换结果 = %q，期望 %q", r.ID, r.Text, wantText[r.ID])
		}
		if r.ID == 3 && r.Type != "null" {
			t.Fatalf("id=3 应保持 NULL，实际类型 %q", r.Type)
		}
		if r.ID != 3 && r.Type != "integer" {
			t.Fatalf("id=%d 应为 integer，实际类型 %q", r.ID, r.Type)
		}
	}

	// 幂等：再跑一次不应再改任何行
	n2, err := normalizeTimeColumns(db)
	if err != nil {
		t.Fatalf("二次归一化失败: %v", err)
	}
	if n2 != 0 {
		t.Fatalf("二次归一化应为幂等（0 行），实际 %d 行", n2)
	}
}

// 无法解析的脏值应转成 NULL，而不是让字符串留在整数列里。
func TestNormalizeTimeColumns_DirtyValueBecomesNull(t *testing.T) {
	db := openLegacyDB(t, func(db *gorm.DB) {
		if err := db.Exec(`CREATE TABLE delivery_logs (
			id INTEGER PRIMARY KEY,
			fired_at datetime
		)`).Error; err != nil {
			t.Fatalf("建旧表失败: %v", err)
		}
		for i, v := range []string{"", "not a time", "2026-06-26 12:00:00+00:00"} {
			if err := db.Exec(`INSERT INTO delivery_logs (id, fired_at) VALUES (?, ?)`, i+1, v).Error; err != nil {
				t.Fatalf("插入失败: %v", err)
			}
		}
	})

	if _, err := normalizeTimeColumns(db); err != nil {
		t.Fatalf("归一化失败: %v", err)
	}

	var rows []struct {
		ID   int    `gorm:"column:id"`
		T    string `gorm:"column:t"`
		Text string `gorm:"column:txt"`
	}
	if err := db.Raw(`SELECT id, typeof(fired_at) AS t, CAST(fired_at AS TEXT) AS txt FROM delivery_logs ORDER BY id`).
		Scan(&rows).Error; err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	for _, r := range rows {
		if r.ID == 3 {
			if r.T != "integer" || r.Text != "1782475200000" {
				t.Fatalf("正常值应转成整数，实际 type=%q value=%q", r.T, r.Text)
			}
			continue
		}
		if r.T != "null" {
			t.Fatalf("id=%d 的脏值应转成 NULL，实际 type=%q value=%q", r.ID, r.T, r.Text)
		}
	}
}

// 升级后新写入的数据，与转换后的历史数据共处同一列，范围查询应同时命中两者。
func TestNormalizeTimeColumns_MixedDataStillQueryable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mixed.db")
	db, err := gorm.Open(sqlite.Open(path), gormConfig())
	if err != nil {
		t.Fatalf("打开数据库失败: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("获取 sql.DB 失败: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	if err := db.AutoMigrate(&models.Reminder{}); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}

	// 直接写一条旧格式文本，模拟升级前遗留的行
	legacy := time.Now().Add(-time.Hour).UTC()
	if err := db.Exec(`INSERT INTO reminders (title, calendar, schedule_type, enabled, next_fire_at) VALUES (?, ?, ?, ?, ?)`,
		"旧行", "solar", "once", true, legacy.Format("2006-01-02 15:04:05.999999999-07:00")).Error; err != nil {
		t.Fatalf("写入旧行失败: %v", err)
	}
	if _, err := normalizeTimeColumns(db); err != nil {
		t.Fatalf("归一化失败: %v", err)
	}

	// 再用新类型写一条
	fresh := time.Now().Add(-2 * time.Hour).UTC()
	if err := db.Create(&models.Reminder{
		Title: "新行", Calendar: "solar", ScheduleType: "once", Enabled: true,
		NextFireAt: models.TimePtrOf(fresh),
	}).Error; err != nil {
		t.Fatalf("写入新行失败: %v", err)
	}

	// 截止到 30 分钟前：两行都该命中，且按时间升序
	var rows []models.Reminder
	if err := db.Where("enabled = ? AND next_fire_at IS NOT NULL AND next_fire_at <= ?", true,
		models.TimeOf(time.Now().Add(-30*time.Minute))).
		Order("next_fire_at ASC").Find(&rows).Error; err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("应同时命中新旧两行，实际 %d 行", len(rows))
	}
	if rows[0].Title != "新行" || rows[1].Title != "旧行" {
		t.Fatalf("排序错误：%q, %q", rows[0].Title, rows[1].Title)
	}
}

// 迁移后 next_fire_at 上的查询应能走索引而不是全表扫描。
func TestReminderDueQueryUsesIndex(t *testing.T) {
	db := openLegacyDB(t, func(db *gorm.DB) {})
	if err := db.AutoMigrate(&models.Reminder{}); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}

	var plan []struct {
		Detail string `gorm:"column:detail"`
	}
	if err := db.Raw(`EXPLAIN QUERY PLAN
		SELECT * FROM reminders
		WHERE enabled = ? AND next_fire_at IS NOT NULL AND next_fire_at <= ?
		ORDER BY next_fire_at ASC LIMIT 10`,
		true, models.TimeOf(time.Now())).Scan(&plan).Error; err != nil {
		t.Fatalf("查询执行计划失败: %v", err)
	}
	if len(plan) == 0 {
		t.Fatal("未取到执行计划")
	}
	// 必须用上索引；若退化成 SCAN reminders 说明索引没生效
	usedIndex := false
	for _, p := range plan {
		t.Logf("计划: %s", p.Detail)
		if strings.HasPrefix(p.Detail, "SEARCH") {
			usedIndex = true
		}
		if strings.Contains(p.Detail, "SCAN reminders") {
			t.Fatalf("查询退化为全表扫描：%s", p.Detail)
		}
	}
	if !usedIndex {
		t.Fatal("查询未用上索引")
	}
}
