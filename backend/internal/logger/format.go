package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// textHandler 输出 new-api 风格的单行日志：
//
//	[INFO] 2026/11/07 - 20:31:39 | 7bc69bc7850af4d0 | 开始投递 reminder=12 log=34 title=喝水
//
// 相比 slog 默认的 TextHandler，做了三处取舍：
//   - request_id 单独占一列：一次请求的多条日志能直接对上，不用在 kv 里找；
//   - 字段值不加引号（除非含空白或特殊字符）：默认格式会给每个字符串都套引号；
//   - 源码位置只在 Warn 及以上输出：INFO 是业务节点，位置信息纯属噪音。
type textHandler struct {
	mu        *sync.Mutex
	w         io.Writer
	level     slog.Leveler
	addSource bool
	attrs     []slog.Attr
	group     string
}

// newTextHandler 构造文本 handler。
//
// opts.Level 为空时按 Info 处理，与 slog 默认值保持一致。
func newTextHandler(w io.Writer, opts *slog.HandlerOptions) *textHandler {
	h := &textHandler{mu: &sync.Mutex{}, w: w, level: slog.LevelInfo}
	if opts != nil {
		if opts.Level != nil {
			h.level = opts.Level
		}
		h.addSource = opts.AddSource
	}
	return h
}

// Enabled 按级别过滤。
func (h *textHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level.Level()
}

// WithAttrs 返回带预置字段的新 handler。
//
// 拷贝结构体但共享同一个互斥锁指针，保证并发写不交错。
func (h *textHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	nh := *h
	nh.attrs = append(append([]slog.Attr{}, h.attrs...), attrs...)
	return &nh
}

// WithGroup 返回带分组前缀的新 handler。
//
// 本项目的日志不使用分组；这里实现成「key 加前缀」，
// 以免某个依赖调用 WithGroup 后字段被静默丢弃。
func (h *textHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	nh := *h
	if nh.group != "" {
		nh.group += "."
	}
	nh.group += name
	return &nh
}

// Handle 渲染一条日志。
func (h *textHandler) Handle(_ context.Context, r slog.Record) error {
	var b strings.Builder
	b.Grow(160)

	b.WriteByte('[')
	b.WriteString(levelLabel(r.Level))
	b.WriteString("] ")
	b.WriteString(r.Time.Format("2006/01/02 - 15:04:05"))
	b.WriteString(" | ")

	// 先把字段收齐：request_id 要提到消息前面单独成一列
	var requestID string
	fields := make([]slog.Attr, 0, len(h.attrs)+r.NumAttrs()+1)

	appendAttr := func(a slog.Attr) {
		a.Value = a.Value.Resolve()
		if a.Equal(slog.Attr{}) {
			return
		}
		key := a.Key
		if h.group != "" {
			key = h.group + "." + key
		}
		if key == slog.TimeKey || key == slog.LevelKey || key == slog.MessageKey ||
			key == slog.SourceKey || key == "src" {
			return
		}
		if key == "request_id" {
			requestID = a.Value.String()
			return
		}
		fields = append(fields, slog.Attr{Key: key, Value: a.Value})
	}
	for _, a := range h.attrs {
		appendAttr(a)
	}
	r.Attrs(func(a slog.Attr) bool {
		appendAttr(a)
		return true
	})

	if requestID != "" {
		b.WriteString(requestID)
		b.WriteString(" | ")
	}
	b.WriteString(r.Message)

	for _, a := range fields {
		b.WriteByte(' ')
		b.WriteString(a.Key)
		b.WriteByte('=')
		writeValue(&b, a.Value)
	}

	// 只有需要人去看的日志才带源码位置
	if h.addSource && r.Level >= slog.LevelWarn {
		if src := sourceOf(r); src != "" {
			b.WriteString(" src=")
			b.WriteString(src)
		}
	}
	b.WriteByte('\n')

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := io.WriteString(h.w, b.String())
	return err
}

// levelLabel 返回日志级别标签。
func levelLabel(l slog.Level) string {
	switch {
	case l >= slog.LevelError:
		return "ERR"
	case l >= slog.LevelWarn:
		return "WARN"
	case l >= slog.LevelInfo:
		return "INFO"
	default:
		return "DEBUG"
	}
}

// writeValue 写字段值。
//
// 含空白、引号或等号的值加引号，避免和后面的 kv 混在一起读不出来。
func writeValue(b *strings.Builder, v slog.Value) {
	switch v.Kind() {
	case slog.KindString:
		s := v.String()
		if s == "" || strings.ContainsAny(s, " \t\r\n\"=") {
			b.WriteString(strconv.Quote(s))
			return
		}
		b.WriteString(s)
	case slog.KindDuration:
		b.WriteString(v.Duration().String())
	case slog.KindTime:
		b.WriteString(v.Time().Format(time.RFC3339))
	case slog.KindAny:
		if err, ok := v.Any().(error); ok {
			writeValue(b, slog.StringValue(err.Error()))
			return
		}
		writeValue(b, slog.StringValue(fmt.Sprint(v.Any())))
	default:
		writeValue(b, slog.StringValue(v.String()))
	}
}

// sourceOf 返回缩短后的源码位置（internal/xxx.go:42）。
//
// 与 slog 默认的 FILE:LINE 相比，这里保留 internal/ 之后的相对路径：
// 部署机器上绝对路径没有意义，而且会把一行日志撑得很长。
func sourceOf(r slog.Record) string {
	if r.PC == 0 {
		return ""
	}
	frames := runtime.CallersFrames([]uintptr{r.PC})
	frame, _ := frames.Next()
	return shortenSource(frame.File, frame.Line)
}

// shortenSource 把绝对路径裁剪到 internal/ 之后。
func shortenSource(file string, line int) string {
	if idx := strings.Index(file, "internal/"); idx >= 0 {
		file = file[idx:]
	}
	return file + ":" + strconv.Itoa(line)
}
