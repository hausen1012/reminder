// Package logger 提供全局 slog 初始化与按日滚动日志配置。
package logger

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// 文件级别的日志目录写失败时，重复告警会淹没有用信息；只提示一次。
var writerWarnOnce sync.Once

// activeCloser 记录当前打开的文件 writer，供重新 Init 时释放资源。
var activeCloser struct {
	mu sync.Mutex
	c  io.Closer
}

// activeLevel 保存 Init 时确定的级别，供 Level() 读取（atomic：GORM 适配会并发读）。
var activeLevel atomic.Int32

func init() {
	activeLevel.Store(int32(slog.LevelInfo))
}

// Level 返回 Init 时确定的日志级别，供其他包（如 GORM 日志适配）对齐。
func Level() slog.Level {
	return slog.Level(activeLevel.Load())
}

// Init 配置全局 slog logger；logDir 非空时同时写入按日滚动的文件，可重复调用。
func Init(logDir string) {
	// 关闭上一次 Init 打开的文件：全局 logger 只保留最新一份
	activeCloser.mu.Lock()
	if activeCloser.c != nil {
		_ = activeCloser.c.Close()
		activeCloser.c = nil
	}
	activeCloser.mu.Unlock()

	var writers []io.Writer
	writers = append(writers, os.Stderr)

	if logDir != "" {
		dw, err := newDailyWriter(logDir, time.Now)
		if err != nil {
			fmt.Fprintf(os.Stderr, "日志目录不可用，仅输出到标准错误: %v\n", err)
		} else {
			writers = append(writers, dw)
			activeCloser.mu.Lock()
			activeCloser.c = dw
			activeCloser.mu.Unlock()
		}
	}

	multi := io.MultiWriter(writers...)

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
		h = slog.NewJSONHandler(multi, opts)
	default:
		h = newTextHandler(multi, opts)
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

// dailyWriter 是按自然日滚动的文件 writer；打开或写入失败时静默降级到标准错误（只提示一次）。
type dailyWriter struct {
	mu   sync.Mutex
	dir  string
	now  func() time.Time
	file *os.File
	day  string
}

func newDailyWriter(dir string, now func() time.Time) (*dailyWriter, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	w := &dailyWriter{dir: dir, now: now}
	if err := w.rotateLocked(w.now()); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *dailyWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if err := w.rotateLocked(w.now()); err != nil {
		writerWarnOnce.Do(func() {
			fmt.Fprintf(os.Stderr, "日志文件不可写，后续仅输出到标准错误: %v\n", err)
		})
		return len(p), nil
	}
	if _, err := w.file.Write(p); err != nil {
		writerWarnOnce.Do(func() {
			fmt.Fprintf(os.Stderr, "写日志文件失败，后续仅输出到标准错误: %v\n", err)
		})
		return len(p), nil
	}
	return len(p), nil
}

// rotateLocked 在日期变化时切换文件；调用方必须持有 w.mu。
func (w *dailyWriter) rotateLocked(now time.Time) error {
	day := now.Format("2006-01-02")
	if w.file != nil && w.day == day {
		return nil
	}
	if w.file != nil {
		_ = w.file.Close()
		w.file = nil
		w.day = ""
	}
	f, err := os.OpenFile(filepath.Join(w.dir, day+".log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	w.file, w.day = f, day
	return nil
}

// Close 关闭当前打开的文件，供重复 Init 时释放句柄。
func (w *dailyWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file, w.day = nil, ""
	return err
}
