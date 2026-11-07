package logger

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// newTestLogger 返回写入 buffer 的 textHandler logger。
func newTestLogger(level slog.Level) (*slog.Logger, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	h := newTextHandler(buf, &slog.HandlerOptions{Level: level, AddSource: true})
	return slog.New(h), buf
}

// 输出形如：
//
//	[INFO] 2026/11/07 - 20:31:39 | 7bc69bc7 | 开始投递 reminder=12
func TestTextHandler_Shape(t *testing.T) {
	log, buf := newTestLogger(slog.LevelInfo)
	log.InfoContext(context.Background(), "开始投递", "reminder", 12)

	line := strings.TrimSuffix(buf.String(), "\n")
	if !strings.HasPrefix(line, "[INFO] ") {
		t.Fatalf("应以 [INFO] 开头: %q", line)
	}
	// 时间与消息之间用 " | " 分隔
	if !strings.Contains(line, " - ") || !strings.Contains(line, " | 开始投递") {
		t.Fatalf("时间格式或分隔符不对: %q", line)
	}
	if !strings.Contains(line, "reminder=12") {
		t.Fatalf("缺少字段: %q", line)
	}
	if strings.Count(line, "\n") != 0 {
		t.Fatalf("应只输出一行: %q", line)
	}
}

// request_id 要提到消息之前单独成一列，便于按请求串日志。
func TestTextHandler_RequestIDColumn(t *testing.T) {
	log, buf := newTestLogger(slog.LevelInfo)
	log.Info("请求返回业务错误", "request_id", "abc123", "status", 400)

	line := buf.String()
	want := " | abc123 | 请求返回业务错误"
	if !strings.Contains(line, want) {
		t.Fatalf("request_id 应位于消息之前（期望包含 %q）:\n%s", want, line)
	}
	// 不应再出现在 kv 末尾
	if strings.Contains(line, "request_id=abc123") {
		t.Fatalf("request_id 不应重复出现在 kv 中:\n%s", line)
	}
}

// 带空格的值要加引号，否则 kv 会串成一团。
func TestTextHandler_QuotesSpaces(t *testing.T) {
	log, buf := newTestLogger(slog.LevelInfo)
	log.Info("测试", "msg_text", "hello world", "plain", "ok")

	line := buf.String()
	if !strings.Contains(line, `msg_text="hello world"`) {
		t.Fatalf("含空格的值应加引号:\n%s", line)
	}
	if !strings.Contains(line, "plain=ok") {
		t.Fatalf("普通值不应加引号:\n%s", line)
	}
}

// error 值直接打印可读文本，而不是结构化的 slog.Attr 嵌套。
func TestTextHandler_ErrorValue(t *testing.T) {
	log, buf := newTestLogger(slog.LevelWarn)
	log.Warn("失败", "error", errors.New("boom"))

	if got := buf.String(); !strings.Contains(got, "error=boom") {
		t.Fatalf("error 值应输出文本:\n%s", got)
	}
}

// Info 级别不带 src；Warn 及以上带（便于定位，又不让信息日志变长）。
func TestTextHandler_SourceOnlyForWarnUp(t *testing.T) {
	log, buf := newTestLogger(slog.LevelDebug)
	log.Info("info 行")
	log.Warn("warn 行")

	lines := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("应有 2 行，实际 %d:\n%s", len(lines), buf.String())
	}
	if strings.Contains(lines[0], "src=") {
		t.Fatalf("INFO 不应带 src: %q", lines[0])
	}
	if !strings.Contains(lines[1], "src=internal/logger/") {
		t.Fatalf("WARN 应带缩短后的 src: %q", lines[1])
	}
}

// 级别过滤：Info 级别下 Debug 不输出。
func TestTextHandler_LevelFiltering(t *testing.T) {
	log, buf := newTestLogger(slog.LevelInfo)
	log.Debug("不可见")
	log.Info("可见")

	out := buf.String()
	if strings.Contains(out, "不可见") {
		t.Fatalf("应过滤 Debug:\n%s", out)
	}
	if !strings.Contains(out, "可见") {
		t.Fatalf("应保留 Info:\n%s", out)
	}
}

// With 附加的字段要出现在每条日志里。
func TestTextHandler_WithAttrs(t *testing.T) {
	log, buf := newTestLogger(slog.LevelInfo)
	log.With("provider", "openai").Info("对话完成", "rounds", 2)

	out := buf.String()
	if !strings.Contains(out, "provider=openai") || !strings.Contains(out, "rounds=2") {
		t.Fatalf("With 字段应出现在日志中:\n%s", out)
	}
}

// 级别标签映射。
func TestLevelLabel(t *testing.T) {
	cases := map[slog.Level]string{
		slog.LevelDebug: "DEBUG",
		slog.LevelInfo:  "INFO",
		slog.LevelWarn:  "WARN",
		slog.LevelError: "ERR",
	}
	for level, want := range cases {
		if got := levelLabel(level); got != want {
			t.Fatalf("levelLabel(%v) = %q，期望 %q", level, got, want)
		}
	}
}

// 时间字段按 2006/01/02 - 15:04:05 渲染。
func TestTextHandler_TimeFormat(t *testing.T) {
	buf := &bytes.Buffer{}
	h := newTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	r := slog.NewRecord(time.Date(2026, 11, 7, 20, 31, 39, 0, time.UTC), slog.LevelInfo, "msg", 0)
	if err := h.Handle(context.Background(), r); err != nil {
		t.Fatalf("Handle 失败: %v", err)
	}
	if !strings.Contains(buf.String(), "2026/11/07 - 20:31:39") {
		t.Fatalf("时间格式不对:\n%s", buf.String())
	}
}
