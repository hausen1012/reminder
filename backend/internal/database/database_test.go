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
// models.Time 的 Value 会自己归一化，但 GORM 自动填充 created_at / updated_at
// 时直接拿 NowFunc 的返回值；保持 UTC 让自动字段与手写字段走同一条路径，
// 也避免任何直接读 NowFunc 的代码拿到本地偏移。
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

// 自动时间戳必须以整数毫秒落库，与手写字段的存储形态保持一致。
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

	next := time.Now()
	r := &models.Reminder{
		Title:         "时间戳测试",
		Calendar:      "solar",
		ScheduleType:  "once",
		Enabled:       true,
		NextFireAt:    models.TimePtrOf(next),
		ScheduleSpec:  []byte(`{"at":"2099-01-01T09:00:00"}`),
		ContentFormat: "text",
	}
	if err := db.Create(r).Error; err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	// 断言原始列必须是整数毫秒（旧实现里这里读的是驱动转出的 RFC3339 文本，
	// 断言以 Z 结尾其实测不到真实存储格式；改成 CAST 成 INTEGER 才能验证）
	var raw struct {
		CreatedAt *int64 `gorm:"column:created_at"`
	}
	if err := db.Raw("SELECT CAST(created_at AS INTEGER) AS created_at FROM reminders WHERE id = ?", r.ID).
		Scan(&raw).Error; err != nil {
		t.Fatalf("读取原始值失败: %v", err)
	}
	if raw.CreatedAt == nil {
		t.Fatal("created_at 应为整数，实际为 NULL")
	}
	if *raw.CreatedAt <= 0 {
		t.Fatalf("created_at 落库值 = %d，期望正的 Unix 毫秒", *raw.CreatedAt)
	}
	// 毫秒量级：和当前时间的偏差应在一天内
	if diff := time.Since(time.UnixMilli(*raw.CreatedAt)); diff > 24*time.Hour || diff < -24*time.Hour {
		t.Fatalf("created_at 落库值 %d 与当前时间相差 %v，不像 Unix 毫秒", *raw.CreatedAt, diff)
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
