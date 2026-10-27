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

// gormConfig 的 NowFunc 必须返回 UTC。
//
// SQLite 把时间存成带偏移的文本并按字典序比较，站点时区又能在设置页热更新；
// 只要新旧行偏移不一致，created_at 排序与范围查询就会错序。
func TestGormConfig_NowFuncUTC(t *testing.T) {
	// 把进程时区设成非 UTC，模拟生产环境里 TZ=Asia/Shanghai 的部署
	old := time.Local
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatalf("加载时区失败: %v", err)
	}
	time.Local = loc
	t.Cleanup(func() { time.Local = old })

	now := gormConfig().NowFunc()
	if _, offset := now.Zone(); offset != 0 {
		t.Fatalf("NowFunc 偏移 = %d 秒，期望 0（UTC）", offset)
	}
}

// 自动时间戳落库后必须是 UTC 文本，与手写字段的 UTC 约定一致。
func TestAutoTimestampStoredAsUTC(t *testing.T) {
	old := time.Local
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatalf("加载时区失败: %v", err)
	}
	time.Local = loc
	t.Cleanup(func() { time.Local = old })

	path := filepath.Join(t.TempDir(), "ts.db")
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

	next := time.Now().UTC()
	r := &models.Reminder{
		Title:         "时间戳测试",
		Calendar:      "solar",
		ScheduleType:  "once",
		Enabled:       true,
		NextFireAt:    &next,
		ScheduleSpec:  []byte(`{"at":"2099-01-01T09:00:00"}`),
		ContentFormat: "text",
	}
	if err := db.Create(r).Error; err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	var raw struct {
		CreatedAt string `gorm:"column:created_at"`
	}
	if err := db.Raw("SELECT created_at FROM reminders WHERE id = ?", r.ID).Scan(&raw).Error; err != nil {
		t.Fatalf("读取原始值失败: %v", err)
	}
	if !strings.HasSuffix(raw.CreatedAt, "Z") {
		t.Fatalf("created_at 落库值 = %q，期望以 Z 结尾的 UTC 文本", raw.CreatedAt)
	}
}

// 构建 DSN 时保留用户已写的查询串，否则追加 SQLite 并发参数。
func TestBuildDSN(t *testing.T) {
	if got := buildDSN("/tmp/a.db"); !strings.Contains(got, "_journal_mode=WAL") {
		t.Fatalf("未追加并发参数: %q", got)
	}
	const custom = "/tmp/a.db?cache=shared"
	if got := buildDSN(custom); got != custom {
		t.Fatalf("已带查询串时不应改写，得到 %q", got)
	}
}
