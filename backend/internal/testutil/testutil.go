// Package testutil 汇集各包测试共用的脚手架（仅被 *_test.go 引用）。
//
// models 包因导入方向（testutil → database → models）无法引用本包，其用例自带建库辅助函数。
package testutil

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/reminder/backend/internal/database"
	"github.com/reminder/backend/internal/timezone"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// nameSeq 给测试造数提供进程内单调递增的序号。
var nameSeq atomic.Uint64

// UniqueName 返回一个测试内唯一的名字，用于通道等带唯一约束的字段。
//
// 不能只用 time.Now().UnixNano()：Windows 的时钟精度约 15ms，
// 内存库写入又极快，连续两次调用很容易拿到同一个纳秒值而撞唯一约束。
func UniqueName(prefix string) string {
	return fmt.Sprintf("%s-%d-%d", prefix, nameSeq.Add(1), time.Now().UnixNano())
}

// CST 返回测试默认时区（东八区），与生产默认值一致。
func CST() *time.Location {
	return time.FixedZone("CST", 8*3600)
}

// TZ 把固定时区包装成 timezone.Provider，供构造服务时注入。
func TZ(loc *time.Location) *timezone.Provider {
	return timezone.NewFixed(loc)
}

// NewDB 构造测试专用的内存 SQLite 库，并按生产的模型清单建表。
//
// DSN 带上 t.Name()，同包内不同用例互不串库；连接数设为 1，
// SQLite 写串行化，可避免 "database is locked"。
func NewDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{
		// 与生产一致（database.gormConfig）：自动填充的时间也取 UTC，
		// 落库仍由 models.Time 归一化成整数毫秒
		NowFunc: func() time.Time { return time.Now().UTC() },
	})
	if err != nil {
		t.Fatalf("打开内存数据库失败: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("获取 sql.DB 失败: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	if err := db.AutoMigrate(database.AllModels()...); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	return db
}
