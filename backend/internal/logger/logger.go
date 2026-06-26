// Package logger 提供全局 slog 配置。
//
// 开发环境输出 TextHandler（带颜色/源码行号），生产环境输出 JSONHandler。
// 可通过环境变量 LOG_FORMAT=json 强制 JSON 格式。
// 日志同时写入控制台和文件（如果 cfg.LogFile 配置了）。
//
// 使用方式：
//
//	import "github.com/reminder/backend/internal/logger"
//
//	logger.Init(cfg)
//	slog.Info("server starting", "port", cfg.Port)
//
// 所有模块统一使用 slog 标准库，不再使用 log.Printf / fmt.Printf 输出日志。
package logger

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

// Init 配置全局 slog logger。
// 如果 logDir 非空，日志会同时写入该目录下的按日滚动的文件。
func Init(logDir string) {
	// 多写器：stderr + 文件
	var writers []io.Writer
	writers = append(writers, os.Stderr)

	if logDir != "" {
		if err := os.MkdirAll(logDir, 0755); err == nil {
			logPath := filepath.Join(logDir, time.Now().Format("2006-01-02")+".log")
			f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
			if err == nil {
				writers = append(writers, f)
			}
		}
	}

	multi := io.MultiWriter(writers...)

	// 默认 TextHandler（开发友好），LOG_FORMAT=json 时切换 JSONHandler
	var h slog.Handler
	switch os.Getenv("LOG_FORMAT") {
	case "json":
		h = slog.NewJSONHandler(multi, &slog.HandlerOptions{
			Level:     slog.LevelInfo,
			AddSource: true,
		})
	default:
		h = slog.NewTextHandler(multi, &slog.HandlerOptions{
			Level:     slog.LevelInfo,
			AddSource: true,
		})
	}

	slog.SetDefault(slog.New(h))
}