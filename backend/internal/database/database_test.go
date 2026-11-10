package database

import (
	"bytes"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/reminder/backend/internal/config"
	"github.com/reminder/backend/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
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

	// 断言原始列必须是整数毫秒存储
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

// captureSlog 把全局 slog 临时重定向到内存 buffer。
func captureSlog(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(old) })
	return buf
}

// LOG_LEVEL 到 GORM 级别的映射。
func TestGormLogLevel(t *testing.T) {
	cases := []struct {
		in   slog.Level
		want gormlogger.LogLevel
	}{
		{slog.LevelDebug, gormlogger.Info},
		{slog.LevelInfo, gormlogger.Warn},
		{slog.LevelWarn, gormlogger.Error},
		{slog.LevelError, gormlogger.Error},
	}
	for _, tc := range cases {
		if got := gormLogLevel(tc.in); got != tc.want {
			t.Fatalf("gormLogLevel(%v) = %v，期望 %v", tc.in, got, tc.want)
		}
	}
}

// 失败 SQL 应以 Error 级别进结构化日志。
func TestGormConfig_LogsFailedSQL(t *testing.T) {
	logs := captureSlog(t)

	cfg := gormConfig()
	cfg.Logger = cfg.Logger.LogMode(gormlogger.Info)

	path := filepath.Join(t.TempDir(), "log.db")
	db, err := gorm.Open(sqlite.Open(path), cfg)
	if err != nil {
		t.Fatalf("打开数据库失败: %v", err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })

	if err := db.Raw("SELECT * FROM no_such_table").Scan(&[]models.Reminder{}).Error; err == nil {
		t.Fatal("查询不存在的表应失败")
	}

	out := logs.String()
	if !strings.Contains(out, "level=ERROR") || !strings.Contains(out, "no_such_table") {
		t.Fatalf("失败 SQL 应写入 slog 的 ERROR 日志:\n%s", out)
	}
}

// 「查不到记录」是正常业务分支，不应被记成 SQL 错误。
func TestGormConfig_IgnoresRecordNotFound(t *testing.T) {
	logs := captureSlog(t)

	cfg := gormConfig()
	cfg.Logger = cfg.Logger.LogMode(gormlogger.Info)

	path := filepath.Join(t.TempDir(), "nf.db")
	db, err := gorm.Open(sqlite.Open(path), cfg)
	if err != nil {
		t.Fatalf("打开数据库失败: %v", err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })

	if err := db.AutoMigrate(&models.Reminder{}); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}

	var r models.Reminder
	if err := db.First(&r, 9999).Error; err == nil {
		t.Fatal("不存在的记录应返回错误")
	}

	if out := logs.String(); strings.Contains(out, "level=ERROR") {
		t.Fatalf("RecordNotFound 不应记为 ERROR:\n%s", out)
	}
}

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

	var users int64
	if err := DB.Model(&models.User{}).Count(&users).Error; err != nil {
		t.Fatalf("统计用户失败: %v", err)
	}
	if users != 1 {
		t.Fatalf("应创建 1 个默认管理员，实际 %d", users)
	}
}

// Init 遇到旧库中含有 enabled 列的 channels 表时，应平滑删除该列且不 panic。
func TestInit_DropsLegacyEnabledColumn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	cfg := &config.Config{DBPath: path, Username: "admin", Password: "admin123"}

	preDB, err := gorm.Open(sqlite.Open(buildDSN(path)), gormConfig())
	if err != nil {
		t.Fatalf("预建库失败: %v", err)
	}
	if err := preDB.Exec("CREATE TABLE channels (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, type TEXT, enabled NUMERIC, config TEXT, created_at INTEGER, updated_at INTEGER);").Error; err != nil {
		t.Fatalf("创建旧 channels 表失败: %v", err)
	}
	preSqlDB, _ := preDB.DB()
	_ = preSqlDB.Close()

	if err := Init(cfg); err != nil {
		t.Fatalf("Init 失败: %v", err)
	}
	sqlDB, err := DB.DB()
	if err != nil {
		t.Fatalf("获取连接失败: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	if DB.Migrator().HasColumn("channels", "enabled") {
		t.Fatal("channels 表中仍残留 enabled 列")
	}
}
