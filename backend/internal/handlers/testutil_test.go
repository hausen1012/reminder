package handlers

import (
	"testing"

	"github.com/reminder/backend/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newTestDB 构造一个仅供测试使用的内存 SQLite 数据库。
//
// 之前 handler 直接依赖 database.DB 全局单例，无法在无真实库的情况下测试；
// 现改为构造服务时注入 *gorm.DB，这里就可以用内存库跑 handler 层测试。
func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	// file:...?mode=memory&cache=shared 让同一连接串在测试期间共享同一份内存库
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"),
		&gorm.Config{})
	if err != nil {
		t.Fatalf("打开内存数据库失败: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("获取 sql.DB 失败: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	if err := db.AutoMigrate(
		&models.User{},
		&models.Reminder{},
		&models.ReminderChannel{},
		&models.Channel{},
		&models.DeliveryLog{},
		&models.DeliveryAttempt{},
		&models.Token{},
		&models.TokenDefaultChannel{},
		&models.ConfirmToken{},
	); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	return db
}
