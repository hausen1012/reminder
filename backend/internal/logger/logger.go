// Package logger 提供全局 slog 配置。
//
// 开发环境输出 TextHandler（带颜色/源码行号），生产环境输出 JSONHandler。
// 可通过环境变量 LOG_FORMAT=json 强制 JSON 格式，LOG_LEVEL 调整最低级别。
// 日志同时写入控制台和文件（如果 logDir 非空），文件按进程时区按日滚动
// （启动时先按 TIMEZONE 对齐，router 初始化后再对齐库里的站点时区）。
//
// 使用方式：
//
//	import "github.com/reminder/backend/internal/logger"
//
//	logger.Init(cfg.LogFile)
//	slog.Info("server starting", "port", cfg.Port)
//
// 所有模块统一使用 slog 标准库，不再使用 log.Printf / fmt.Printf 输出日志。
package logger

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// 文件级别的日志目录写失败时，重复告警会淹没有用信息；只提示一次。
var writerWarnOnce sync.Once

// activeCloser 是上一次 Init 打开的文件 writer。
//
// 重复 Init（测试里常见）会换掉全局 logger，若不关闭旧文件句柄，
// Windows 上临时目录无法删除；长期运行也会累积句柄。
var activeCloser struct {
	mu sync.Mutex
	c  io.Closer
}

// activeLevel 保存 Init 时确定的级别，供 Level() 读取。
//
// 用 atomic 而不是普通变量：GORM 日志适配可能在并发请求里读它。
var activeLevel atomic.Int32

func init() {
	activeLevel.Store(int32(slog.LevelInfo))
}

// Level 返回 Init 时确定的日志级别（由 LOG_LEVEL 决定）。
//
// 供其他包（如 GORM 的 SQL 日志适配）对齐级别：日志级别只解析一次，
// 避免两套解析逻辑对同一个值给出不同结论。
func Level() slog.Level {
	return slog.Level(activeLevel.Load())
}

// Init 配置全局 slog logger。
//
// 如果 logDir 非空，日志会同时写入该目录下按日滚动的文件。
// 级别取自环境变量 LOG_LEVEL（debug / info / warn / error），默认 info。
// 可重复调用：每次调用都会重新设置默认 logger。
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

	// 统一时间格式：xxxx-xx-xx HH:mm:ss，source 缩短为 package/file.go:N
	replace := func(_ []string, a slog.Attr) slog.Attr {
		switch a.Key {
		case slog.TimeKey:
			return slog.String("time", a.Value.Time().Format("2006-01-02 15:04:05"))
		case slog.SourceKey:
			if src, ok := a.Value.Any().(*slog.Source); ok && src != nil {
				if idx := strings.Index(src.File, "internal/"); idx >= 0 {
					return slog.String("src", src.File[idx:]+":"+strconv.Itoa(src.Line))
				}
				return slog.String("src", src.File+":"+strconv.Itoa(src.Line))
			}
		}
		return a
	}
	level := parseLevel(os.Getenv("LOG_LEVEL"))
	activeLevel.Store(int32(level))
	opts := &slog.HandlerOptions{
		Level:       level,
		AddSource:   true,
		ReplaceAttr: replace,
	}

	// 默认 TextHandler（开发友好），LOG_FORMAT=json 时切换 JSONHandler
	var h slog.Handler
	switch strings.ToLower(strings.TrimSpace(os.Getenv("LOG_FORMAT"))) {
	case "json":
		h = slog.NewJSONHandler(multi, opts)
	default:
		h = slog.NewTextHandler(multi, opts)
	}

	slog.SetDefault(slog.New(h))
}

// parseLevel 把 LOG_LEVEL 转成 slog.Level。
//
// 空值或无法识别的值一律回退到 info；logger 尚未就绪，因此直接写标准错误。
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

// dailyWriter 是按自然日滚动的文件 writer。
//
// 每次写入前检查日期，跨天时关闭旧文件并打开新文件。打开失败时不阻塞日志：
// 调用方已经拿到标准错误输出，这里只补充一次提示，避免把写日志本身变成故障源。
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
