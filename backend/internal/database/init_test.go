package database

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/reminder/backend/internal/config"
	"github.com/reminder/backend/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// 端到端升级：直接用 Init 打开一个旧版本形态的库，
// 验证启动流程会自动把时间列转换成整数毫秒，且数据可查、可排序。
func TestInit_MigratesLegacyDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")

	// 1) 先手工造一个旧版本库（文本时间 + 旧索引 + 一条历史数据）
	legacy, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatalf("创建旧库失败: %v", err)
	}
	if err := legacy.Exec(legacyRemindersDDL).Error; err != nil {
		t.Fatalf("建旧表失败: %v", err)
	}
	base := time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)
	cst := base.In(time.FixedZone("CST", 8*3600))
	for _, v := range []string{
		base.Format("2006-01-02 15:04:05.999999999-07:00"),
		cst.Format("2006-01-02 15:04:05.999999999-07:00"),
	} {
		if err := legacy.Exec(`INSERT INTO reminders (title, calendar, schedule_type, enabled, source, next_fire_at, created_at, updated_at, fire_count)
			VALUES ('历史提醒', 'solar', 'once', 1, 'web', ?, ?, ?, 0)`,
			v, v, v).Error; err != nil {
			t.Fatalf("插入历史数据失败: %v", err)
		}
	}
	legacySQL, err := legacy.DB()
	if err != nil {
		t.Fatalf("获取旧库连接失败: %v", err)
	}
	if err := legacySQL.Close(); err != nil {
		t.Fatalf("关闭旧库失败: %v", err)
	}

	// 2) 走真实的启动流程
	cfg := &config.Config{
		DBPath:   path,
		Username: "admin",
		Password: "admin123",
	}
	if err := Init(cfg); err != nil {
		t.Fatalf("Init 失败: %v", err)
	}
	sqlDB, err := DB.DB()
	if err != nil {
		t.Fatalf("获取连接失败: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	// 3) 历史数据的时间列应已转成整数毫秒
	var rows []models.Reminder
	if err := DB.Order("id ASC").Find(&rows).Error; err != nil {
		t.Fatalf("读取历史数据失败: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("应读到 2 条历史提醒，实际 %d", len(rows))
	}
	if rows[0].NextFireAt == nil || rows[1].NextFireAt == nil {
		t.Fatalf("历史数据的时间丢失：%v / %v", rows[0].NextFireAt, rows[1].NextFireAt)
	}
	if !rows[0].NextFireAt.Equal(rows[1].NextFireAt.Std()) {
		t.Fatalf("同一时刻的两种偏移转换后应相等：%v / %v",
			rows[0].NextFireAt.Time, rows[1].NextFireAt.Time)
	}
	if !rows[0].NextFireAt.Equal(base) {
		t.Fatalf("时间值转换错误：%v，期望 %v", rows[0].NextFireAt.Time, base)
	}

	var textLeft int64
	if err := DB.Raw(`SELECT COUNT(*) FROM reminders WHERE typeof(next_fire_at) = 'text'`).
		Scan(&textLeft).Error; err != nil {
		t.Fatalf("检查列类型失败: %v", err)
	}
	if textLeft != 0 {
		t.Fatalf("仍有 %d 行时间列是文本", textLeft)
	}

	// 4) 默认管理员已创建（旧库里没有用户）
	var users int64
	if err := DB.Model(&models.User{}).Count(&users).Error; err != nil {
		t.Fatalf("统计用户失败: %v", err)
	}
	if users != 1 {
		t.Fatalf("应创建 1 个默认管理员，实际 %d", users)
	}
}

// Init 对全新库同样可用，且创建的库时间列全是整数。
func TestInit_FreshDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fresh.db")
	cfg := &config.Config{DBPath: path, Username: "admin", Password: "admin123"}
	if err := Init(cfg); err != nil {
		t.Fatalf("Init 失败: %v", err)
	}
	sqlDB, err := DB.DB()
	if err != nil {
		t.Fatalf("获取连接失败: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	// 每个时间列都应是 INTEGER
	cols, err := discoverTimeColumns(DB)
	if err != nil {
		t.Fatalf("发现时间列失败: %v", err)
	}
	if len(cols) == 0 {
		t.Fatal("未发现任何时间列，模型清单可能没更新")
	}
	for _, c := range cols {
		var typ string
		if err := DB.Raw("SELECT type FROM pragma_table_info(?) WHERE name = ?", c.Table, c.Column).
			Scan(&typ).Error; err != nil {
			t.Fatalf("读取 %s.%s 类型失败: %v", c.Table, c.Column, err)
		}
		if typ != "INTEGER" && typ != "integer" {
			t.Fatalf("%s.%s 类型 = %q，期望 INTEGER", c.Table, c.Column, typ)
		}
	}
}
