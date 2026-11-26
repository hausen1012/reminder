// Package logger 提供全局 slog 初始化。
package logger

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync/atomic"
)

// activeLevel 保存 Init 时确定的级别，供 Level() 读取（atomic：GORM 适配会并发读）。
var activeLevel atomic.Int32

func init() {
	activeLevel.Store(int32(slog.LevelInfo))
}

// Level 返回 Init 时确定的日志级别，供其他包（如 GORM 日志适配）对齐。
func Level() slog.Level {
	return slog.Level(activeLevel.Load())
}

// Init 配置全局 slog logger，所有日志仅输出到标准错误。
func Init() {
	// 统一时间格式与源码位置：JSON 走 ReplaceAttr，text 由 textHandler 自行排版
	replace := func(_ []string, a slog.Attr) slog.Attr {
		switch a.Key {
		case slog.TimeKey:
			return slog.String("time", a.Value.Time().Format("2006-01-02 15:04:05"))
		case slog.SourceKey:
			if src, ok := a.Value.Any().(*slog.Source); ok && src != nil {
				return slog.String("src", shortenSource(src.File, src.Line))
			}
		}
		return a
	}
	level := parseLevel(os.Getenv("LOG_LEVEL"))
	activeLevel.Store(int32(level))
	opts := &slog.HandlerOptions{
		Level:     level,
		AddSource: true,
		// ReplaceAttr 只对 JSONHandler 生效，textHandler 自行排版
		ReplaceAttr: replace,
	}

	// 默认文本格式，LOG_FORMAT=json 时切换 JSONHandler
	var h slog.Handler
	switch strings.ToLower(strings.TrimSpace(os.Getenv("LOG_FORMAT"))) {
	case "json":
		h = slog.NewJSONHandler(os.Stderr, opts)
	default:
		h = newTextHandler(os.Stderr, opts)
	}

	slog.SetDefault(slog.New(h))
}

// parseLevel 把 LOG_LEVEL 转成 slog.Level；空值或无法识别时回退 info 并提示到标准错误。
func parseLevel(v string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "info":
		return slog.LevelInfo
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		fmt.Fprintf(os.Stderr, "未知的 LOG_LEVEL=%q，已回退为 info\n", v)
		return slog.LevelInfo
	}
}
