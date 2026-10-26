package scheduler

import (
	"context"
	"sync"
	"testing"

	"github.com/reminder/backend/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// fakeDispatcher 记录 Engine 交出去的投递任务，便于断言。
type fakeDispatcher struct {
	mu   sync.Mutex
	runs []dispatchCall
}

type dispatchCall struct {
	reminderID uint
	logID      uint
}

func (f *fakeDispatcher) Run(_ context.Context, r *models.Reminder, deliveryLogID uint) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.runs = append(f.runs, dispatchCall{reminderID: r.ID, logID: deliveryLogID})
}

func (f *fakeDispatcher) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.runs)
}

// openSchedulerTestDB 构造调度器测试用的内存数据库。
func openSchedulerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("打开测试数据库失败: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("获取 sql.DB 失败: %v", err)
	}
	// SQLite 写串行化，单连接避免 database is locked
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	if err := db.AutoMigrate(
		&models.Reminder{},
		&models.ReminderChannel{},
		&models.Channel{},
		&models.DeliveryLog{},
		&models.DeliveryAttempt{},
		&models.ConfirmToken{},
	); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	return db
}
