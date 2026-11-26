// Package config 解析环境变量并提供运行时配置默认值。
package config

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"os"
	"strconv"
)

// Config 汇总所有运行时配置。
type Config struct {
	// 基础配置
	Port      string
	DBPath    string
	JWTSecret string
	Username  string
	Password  string

	// 安全与存储配置
	EncryptionKey    string // base64 编码的 32 字节 AES-256 密钥；为空则使用内置 fallback（不安全）
	LogAutoPurgeDays int    // 投递日志自动清理保留天数；0 表示不自动清理
}

func Load() *Config {
	cfg := &Config{
		Port:      getEnv("PORT", "8765"),
		DBPath:    getEnv("DB_PATH", "/data/db/reminder.db"),
		JWTSecret: getEnv("JWT_SECRET", ""),
		Username:  getEnv("USERNAME", "admin"),
		Password:  getEnv("PASSWORD", "admin123"),

		EncryptionKey:    getEnv("ENCRYPTION_KEY", ""),
		LogAutoPurgeDays: getEnvInt("LOG_AUTO_PURGE_DAYS", 0),
	}

	// JWTSecret 如果留空就自动生成
	if cfg.JWTSecret == "" {
		cfg.JWTSecret = randomSecret()
		slog.Warn("JWT_SECRET 未配置，已自动生成")
	}

	return cfg
}

// getEnv 读环境变量，未设置或为空时返回 fallback。
func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// getEnvInt 读整数环境变量，缺失或解析失败时返回 fallback。
func getEnvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		slog.Warn("环境变量解析为整数失败，使用默认值", "key", key, "value", v, "fallback", fallback)
		return fallback
	}
	return n
}

// randomSecret 生成 32 字节随机串作为 JWT 密钥。
func randomSecret() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("failed to generate random JWT secret: " + err.Error())
	}
	return hex.EncodeToString(b)
}
