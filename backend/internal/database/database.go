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

// AllModels 返回建表所需的全部模型清单。
//
// 除 Init 外，测试用的内存库也用它建表，避免测试各自维护一份模型列表后与生产脱节。
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

// gormConfig 返回生产环境统一使用的 GORM 配置。
//
// NowFunc 仍然强制 UTC：models.Time 的 Value 会自己归一化，这里保持 UTC
// 是为了让 GORM 自动填充的 created_at / updated_at 与手写字段走同一条路径，
// 也避免任何直接读 NowFunc 的代码拿到本地偏移。
//
// Logger 接到 slog 上：之前 GORM 默认把 SQL 打到标准输出，
// 既绕过了 LOG_FORMAT / LOG_LEVEL，也没有结构化字段。现在的规则是：
//   - 失败 SQL → Error 级别，带 sql / rows / error；
//   - 慢查询（> 500ms）→ Warn 级别；
//   - 其余 SQL 只在 LOG_LEVEL=debug 时记录。
//
// RecordNotFound 不计入错误：业务上「查不到」是正常分支（例如 404），
// 记成 Error 会把真正该看的失败淹掉。
//
// 传入的 slog.Default() 是此刻的默认 logger，因此 logger.Init 必须先于
// database.Init 调用（main 里的顺序就是这样）。
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

// buildDSN 在原始 DB 路径上追加 SQLite 推荐的并发参数。
// 如果用户已经在 DBPath 里写了 ? 查询串，则保留不动。
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
