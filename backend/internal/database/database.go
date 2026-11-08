package database

import (
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/reminder/backend/internal/config"
	"github.com/reminder/backend/internal/logger"
	"github.com/reminder/backend/internal/models"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

var DB *gorm.DB

// allModels 是建表所需的全部模型清单。
var allModels = []any{
	&models.User{},
	&models.Reminder{},
	&models.ReminderChannel{},
	&models.Channel{},
	&models.DeliveryLog{},
	&models.DeliveryAttempt{},
	&models.Token{},
	&models.TokenDefaultChannel{},
	&models.ConfirmToken{},
	&models.Config{},
	&models.AIAction{},
}

// AllModels 返回建表所需的全部模型；测试的内存库也用它建表，避免与生产脱节。
func AllModels() []any {
	return append([]any(nil), allModels...)
}

func Init(cfg *config.Config) error {
	dir := filepath.Dir(cfg.DBPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create db dir: %w", err)
	}

	dsn := buildDSN(cfg.DBPath)
	db, err := gorm.Open(sqlite.Open(dsn), gormConfig())
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}

	// SQLite 写连接串行化，避免 "database is locked"，WAL 下读仍可并发
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("get sql.DB: %w", err)
	}
	sqlDB.SetMaxOpenConns(4)
	sqlDB.SetMaxIdleConns(2)

	if err := db.AutoMigrate(allModels...); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	DB = db
	return ensureAdmin(cfg)
}

// gormConfig 返回统一使用的 GORM 配置：
//   - NowFunc 固定 UTC，与 models.Time 的归一化保持一致
//   - Logger 接到 slog：失败 SQL → Error，慢查询（> 500ms）→ Warn，其余仅 debug
//   - RecordNotFound 不计错误：「查不到」是正常业务分支
//
// logger.Init 必须先于 database.Init 调用。
func gormConfig() *gorm.Config {
	return &gorm.Config{
		NowFunc: func() time.Time { return time.Now().UTC() },
		Logger: gormlogger.NewSlogLogger(slog.Default(), gormlogger.Config{
			SlowThreshold:             500 * time.Millisecond,
			LogLevel:                  gormLogLevel(logger.Level()),
			IgnoreRecordNotFoundError: true,
			Colorful:                  false,
		}),
	}
}

// gormLogLevel 把 slog 级别映射成 GORM 的日志级别。
func gormLogLevel(level slog.Level) gormlogger.LogLevel {
	switch {
	case level <= slog.LevelDebug:
		return gormlogger.Info // Debug：打印全部 SQL
	case level <= slog.LevelInfo:
		return gormlogger.Warn // Info：只打印慢查询与错误
	default:
		return gormlogger.Error // Warn/Error：只打印失败 SQL
	}
}

// buildDSN 在 DB 路径上追加 SQLite 推荐的并发参数；路径里已带查询串时保留不动。
func buildDSN(path string) string {
	if strings.Contains(path, "?") {
		return path
	}
	q := url.Values{}
	q.Set("_journal_mode", "WAL")
	q.Set("_synchronous", "NORMAL")
	q.Set("_busy_timeout", "5000")
	return path + "?" + q.Encode()
}

func ensureAdmin(cfg *config.Config) error {
	var count int64
	if err := DB.Model(&models.User{}).Count(&count).Error; err != nil {
		return fmt.Errorf("count users: %w", err)
	}
	if count > 0 {
		return nil
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(cfg.Password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	user := models.User{
		Username: cfg.Username,
		Password: string(hash),
	}
	return DB.Create(&user).Error
}
