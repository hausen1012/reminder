package config

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"os"
	"strconv"
	"time"
)

// Config 汇总所有运行时配置。
type Config struct {
	// 沿用 bedrock 的基础配置
	Port         string
	DBPath       string
	JWTSecret    string
	InitUsername string
	InitPassword string

	// 提醒助手相关新增配置
	SecretBoxKey         string         // base64 编码的 32 字节密钥；为空则使用 crypto/secretbox 内置 fallback
	PublicBaseURL        string         // 用于拼接 confirm_url 的对外可访问根
	Timezone             string         // 时区字符串，例如 Asia/Shanghai
	Location             *time.Location // 解析后的时区实例
	SweepIntervalSec     int            // 补漏扫描间隔，单位秒
	MissToleranceMinutes int            // 漏触发容忍上限，超过则标记为 expired 不发送
	LogAutoPurgeDays     int            // 自动清理日志的保留天数；0 表示不自动清理
	LogFile              string         // 日志文件路径；为空时输出到 stdout
}

func Load() *Config {
	cfg := &Config{
		Port:         getEnv("PORT", "8080"),
		DBPath:       getEnv("DB_PATH", "/data/db/reminder.db"),
		JWTSecret:    getEnv("JWT_SECRET", ""),
		InitUsername: getEnv("INIT_USERNAME", "admin"),
		InitPassword: getEnv("INIT_PASSWORD", "admin123"),

		SecretBoxKey:         getEnv("SECRET_BOX_KEY", ""),
		PublicBaseURL:        getEnv("PUBLIC_BASE_URL", "http://localhost:8080"),
		Timezone:             getEnv("TIMEZONE", "Asia/Shanghai"),
		SweepIntervalSec:     getEnvInt("SWEEP_INTERVAL_SEC", 60),
		MissToleranceMinutes: getEnvInt("MISS_TOLERANCE_MINUTES", 60),
		LogAutoPurgeDays:     getEnvInt("LOG_AUTO_PURGE_DAYS", 0),
		LogFile:              getEnv("LOG_FILE", ""),
	}

	// 沿用 bedrock 的 JWT 自动生成策略
	if cfg.JWTSecret == "" {
		cfg.JWTSecret = randomSecret()
		log.Printf("JWT_SECRET 未配置，已自动生成")
	}

	loc, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		log.Printf("时区 %q 解析失败，回退到 Asia/Shanghai：%v", cfg.Timezone, err)
		loc, _ = time.LoadLocation("Asia/Shanghai")
	}
	cfg.Location = loc

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
		log.Printf("环境变量 %s 解析为整数失败 %q，使用默认值 %d", key, v, fallback)
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
