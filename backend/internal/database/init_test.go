package database

import (
	"path/filepath"
	"testing"

	"github.com/reminder/backend/internal/config"
	"github.com/reminder/backend/internal/models"
)

// Init 对全新库正常执行 AutoMigrate，并自动创建默认管理员。
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

	// 验证表已创建，时间列类型为 INTEGER
	timeCols := []string{"next_fire_at", "last_fired_at", "created_at", "updated_at"}
	for _, col := range timeCols {
		var typ string
		if err := DB.Raw("SELECT type FROM pragma_table_info('reminders') WHERE name = ?", col).
			Scan(&typ).Error; err != nil {
			t.Fatalf("读取 reminders.%s 类型失败: %v", col, err)
		}
		if typ != "INTEGER" && typ != "integer" {
			t.Fatalf("reminders.%s 类型 = %q，期望 INTEGER", col, typ)
		}
	}

	// 默认管理员已创建
	var users int64
	if err := DB.Model(&models.User{}).Count(&users).Error; err != nil {
		t.Fatalf("统计用户失败: %v", err)
	}
	if users != 1 {
		t.Fatalf("应创建 1 个默认管理员，实际 %d", users)
	}
}
