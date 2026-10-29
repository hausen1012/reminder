// 本包测试脚手架：内存库、固定时区与 fake 替身。
package scheduler

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/reminder/backend/internal/models"
	"github.com/reminder/backend/internal/testutil"
	"github.com/reminder/backend/internal/timezone"
	"gorm.io/gorm"
)

// testTZ 把固定时区包装成 timezone.Provider，供测试注入。
func testTZ(loc *time.Location) *timezone.Provider {
	return testutil.TZ(loc)
}

// cstZone 是测试默认时区（东八区），与生产默认值一致。
func cstZone() *time.Location {
	return testutil.CST()
}

// openSchedulerTestDB 构造调度器测试用的内存数据库。
func openSchedulerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	return testutil.NewDB(t)
}

// fakeDispatcher 记录 Engine 交出去的投递任务，便于断言。
type fakeDispatcher struct {
	mu   sync.Mutex
	runs []dispatchCall
}

// dispatchCall 是一次投递调用的记录。
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
