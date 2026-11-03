package database

import (
	"bytes"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/reminder/backend/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// captureSlog 把全局 slog 临时重定向到内存 buffer。
func captureSlog(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(old) })
	return buf
}

// LOG_LEVEL 到 GORM 级别的映射：debug 打印全部 SQL，info 只打慢查询与错误，warn/error 只打失败 SQL。
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

// 失败 SQL 应以 Error 级别进结构化日志，而不是像以前那样打到标准输出。
func TestGormConfig_LogsFailedSQL(t *testing.T) {
	logs := captureSlog(t)

	cfg := gormConfig()
	// 强制打印全部 SQL，确认失败语句确实经过 slog
	cfg.Logger = cfg.Logger.LogMode(gormlogger.Info)

	path := filepath.Join(t.TempDir(), "log.db")
	db, err := gorm.Open(sqlite.Open(path), cfg)
	if err != nil {
		t.Fatalf("打开数据库失败: %v", err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })

	// 查一张不存在的表：必然失败
	if err := db.Raw("SELECT * FROM no_such_table").Scan(&[]models.Reminder{}).Error; err == nil {
		t.Fatal("查询不存在的表应失败")
	}

	out := logs.String()
	if !strings.Contains(out, "level=ERROR") || !strings.Contains(out, "no_such_table") {
		t.Fatalf("失败 SQL 应写入 slog 的 ERROR 日志:\n%s", out)
	}
}

// 「查不到记录」是正常业务分支（例如 404），不应被记成 SQL 错误。
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
