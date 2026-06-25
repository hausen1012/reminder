package database

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/bedrock/backend/internal/config"
	"github.com/bedrock/backend/internal/models"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

var DB *gorm.DB

func Init(cfg *config.Config) error {
	dir := filepath.Dir(cfg.DBPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create db dir: %w", err)
	}

	dsn := buildDSN(cfg.DBPath)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}

	// SQLite 写连接串行化，避免 "database is locked"，WAL 下读仍可并发
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("get sql.DB: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)

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
		&models.Config{},
	); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	DB = db
	return ensureAdmin(cfg)
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