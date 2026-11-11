// Package config 解析环境变量并提供运行时配置默认值。
package config

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

// 日志目录的默认值，供测试等引用同一份默认值。
const DefaultLogFile = "/var/log/reminder"

// Config 汇总所有运行时配置。
//
// 站点时区由 internal/timezone.Provider 持有；这里的 Timezone 只是首次启动的默认值。
type Config struct {
	// 基础配置
	Port      string
	DBPath    string
	JWTSecret string
	Username  string
	Password  string

	// 提醒助手相关新增配置
	EncryptionKey        string // base64 编码的 32 字节 AES-256 密钥；为空则使用内置 fallback（不安全）
	SiteURL              string // 站点对外地址（来自环境变量 SITE_URL），用于拼接确认链接
	Timezone             string // 站点时区的环境变量默认值，例如 Asia/Shanghai
	SweepIntervalSec     int    // 补漏扫描间隔，单位秒
	MissToleranceMinutes int    // 漏触发容忍上限，超过则标记为 expired 不发送
	LogAutoPurgeDays     int    // 自动清理日志的保留天数；0 表示不自动清理
	LogFile              string // 日志目录；默认 /var/log/reminder，创建失败时仅输出到 stderr

	// 安全相关
	MaxBodyMB        int64    // 单个请求体上限（MB），超出返回 413
	TrustedProxies   []string // 可信反向代理 CIDR 列表；为空表示不信任任何代理头
	LoginMaxAttempts int      // 登录连续失败多少次后锁定，0 表示关闭登录限流
	LoginLockoutMin  int      // 登录锁定时长（分钟）
}

func Load() *Config {
	cfg := &Config{
		Port:      getEnv("PORT", "8765"),
		DBPath:    getEnv("DB_PATH", "/data/db/reminder.db"),
		JWTSecret: getEnv("JWT_SECRET", ""),
		Username:  getEnv("USERNAME", "admin"),
		Password:  getEnv("PASSWORD", "admin123"),

		EncryptionKey:        getEnv("ENCRYPTION_KEY", ""),
		SiteURL:              getEnv("SITE_URL", "http://localhost:8765"),
		Timezone:             getEnv("TIMEZONE", "Asia/Shanghai"),
		SweepIntervalSec:     getEnvInt("SWEEP_INTERVAL_SEC", 60),
		MissToleranceMinutes: getEnvInt("MISS_TOLERANCE_MINUTES", 60),
		LogAutoPurgeDays:     getEnvInt("LOG_AUTO_PURGE_DAYS", 0),
		LogFile:              getEnv("LOG_FILE", DefaultLogFile),

		MaxBodyMB:        int64(getEnvInt("MAX_BODY_MB", 1)),
		TrustedProxies:   splitCSV(getEnv("TRUSTED_PROXIES", "")),
		LoginMaxAttempts: getEnvInt("LOGIN_MAX_ATTEMPTS", 5),
		LoginLockoutMin:  getEnvInt("LOGIN_LOCKOUT_MINUTES", 15),
	}
	if cfg.MaxBodyMB <= 0 {
		// 0 或负数会让所有请求被拒，按未配置处理
		slog.Warn("MAX_BODY_MB 非法，回退为 1 MB", "value", cfg.MaxBodyMB)
		cfg.MaxBodyMB = 1
	}
	if cfg.LoginMaxAttempts < 0 {
		cfg.LoginMaxAttempts = 0
	}
	if cfg.LoginLockoutMin <= 0 {
		cfg.LoginLockoutMin = 15
	}

	// JWTSecret 如果留空就自动生成
	if cfg.JWTSecret == "" {
		cfg.JWTSecret = randomSecret()
		slog.Warn("JWT_SECRET 未配置，已自动生成")
	}

	// 启动期逻辑（日志按天切分等）在读到库里的时区之前就会用到 time.Local
	if loc, err := time.LoadLocation(cfg.Timezone); err != nil {
		slog.Warn("TIMEZONE 解析失败，保持进程默认时区", "timezone", cfg.Timezone, "error", err)
	} else {
		time.Local = loc
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

// splitCSV 把逗号分隔的列表拆成切片，忽略空白项（用于 TRUSTED_PROXIES 等多项配置）。
func splitCSV(v string) []string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// randomSecret 生成 32 字节随机串作为 JWT 密钥。
func randomSecret() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("failed to generate random JWT secret: " + err.Error())
	}
	return hex.EncodeToString(b)
}
