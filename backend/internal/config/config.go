package config

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"os"
	"strconv"
	"time"
)

// Config 汇总所有运行时配置。
//
// 时区不在这里：站点时区存在数据库的 config 表（key=timezone），
// 可在面板「设置」页热更新，由 internal/timezone.Provider 持有。
// 这里的 Timezone 仅是首次启动、库里还没有该配置时的环境变量默认值。
type Config struct {
	// 沿用 bedrock 的基础配置
	Port      string
	DBPath    string
	JWTSecret string
	Username  string
	Password  string

	// 提醒助手相关新增配置
	EncryptionKey        string // base64 编码的 32 字节 AES-256 密钥；为空则使用内置 fallback（不安全）
	BaseURL              string // 用于拼接 confirm_url 的对外可访问根
	Timezone             string // 站点时区的环境变量默认值，例如 Asia/Shanghai
	SweepIntervalSec     int    // 补漏扫描间隔，单位秒
	MissToleranceMinutes int    // 漏触发容忍上限，超过则标记为 expired 不发送
	LogAutoPurgeDays     int    // 自动清理日志的保留天数；0 表示不自动清理
	LogFile              string // 日志目录；默认 /var/log/reminder，创建失败时仅输出到 stderr
}

func Load() *Config {
	cfg := &Config{
		Port:      getEnv("PORT", "8765"),
		DBPath:    getEnv("DB_PATH", "/data/db/reminder.db"),
		JWTSecret: getEnv("JWT_SECRET", ""),
		Username:  getEnv("USERNAME", "admin"),
		Password:  getEnv("PASSWORD", "admin123"),

		EncryptionKey:        getEnv("ENCRYPTION_KEY", "changeme"),
		BaseURL:              getEnv("BASE_URL", "http://localhost:8765"),
		Timezone:             getEnv("TIMEZONE", "Asia/Shanghai"),
		SweepIntervalSec:     getEnvInt("SWEEP_INTERVAL_SEC", 60),
		MissToleranceMinutes: getEnvInt("MISS_TOLERANCE_MINUTES", 60),
		LogAutoPurgeDays:     getEnvInt("LOG_AUTO_PURGE_DAYS", 0),
		LogFile:              getEnv("LOG_FILE", "/var/log/reminder"),
	}

	// JWTSecret 如果留空就自动生成，保持向下兼容
	if cfg.JWTSecret == "" {
		cfg.JWTSecret = randomSecret()
		slog.Warn("JWT_SECRET 未配置，已自动生成")
	}

	// 先把进程时区对齐到环境变量给出的值：日志按天切分等启动期逻辑
	// 在读到库里的站点时区之前就会用到 time.Local。
	// 运行期真正的时区来源是 internal/timezone.Provider（设置页可改）。
	if loc, err := time.LoadLocation(cfg.Timezone); err != nil {
		slog.Warn("TIMEZONE 解析失败，保持进程默认时区", "timezone", cfg.Timezone, "error", err)
	} else {
		time.Local = loc
	}

	return cfg
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

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

func randomSecret() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("failed to generate random JWT secret: " + err.Error())
	}
	return hex.EncodeToString(b)
}
