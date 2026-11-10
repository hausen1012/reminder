package logger

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// newLogDir 建一个临时日志目录，并把「关闭文件 → 删除目录」排到清理函数的最后。
//
// t.Cleanup 是后进先出：先登记目录清理（先于开文件），再登记 resetOnCleanup，
// 清理时就会先关文件再删目录。否则 Windows 上会因文件仍被占用而删除失败。
func newLogDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	resetOnCleanup(t)
	return dir
}

// resetOnCleanup 在用例结束时关掉文件句柄并还原默认 logger。
//
// 必须先 Init("") 关掉文件——Windows 上文件仍被占用时，t.TempDir 的清理会失败。
func resetOnCleanup(t *testing.T) {
	t.Helper()
	old := slog.Default()
	t.Cleanup(func() {
		Init("") // 只写标准错误，同时关闭上次打开的文件
		slog.SetDefault(old)
	})
}

// LOG_LEVEL 的解析：大小写、空白、常见别名都应能识别，未知值回退 info。
func TestParseLevel(t *testing.T) {
	cases := []struct {
		in   string
		want slog.Level
	}{
		{"", slog.LevelInfo},
		{"info", slog.LevelInfo},
		{"INFO", slog.LevelInfo},
		{" debug ", slog.LevelDebug},
		{"Warn", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{"error", slog.LevelError},
		{"verbose", slog.LevelInfo}, // 未知值回退
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := parseLevel(tc.in); got != tc.want {
				t.Fatalf("parseLevel(%q) = %v，期望 %v", tc.in, got, tc.want)
			}
		})
	}
}

// 默认级别是 info：不配 LOG_LEVEL 时 Debug 日志不应输出。
func TestInit_DefaultLevelIsInfo(t *testing.T) {
	t.Setenv("LOG_LEVEL", "")
	t.Setenv("LOG_FORMAT", "text")
	dir := newLogDir(t)

	Init(dir)
	if got := Level(); got != slog.LevelInfo {
		t.Fatalf("Level() = %v，期望 info", got)
	}

	var buf bytes.Buffer
	// 用一个同样级别的 handler 验证：Debug 被过滤，Info 保留
	h := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: Level()})
	log := slog.New(h)
	log.Debug("不可见")
	log.Info("可见")
	out := buf.String()
	if strings.Contains(out, "不可见") {
		t.Fatalf("info 级别下不应输出 debug 日志: %s", out)
	}
	if !strings.Contains(out, "可见") {
		t.Fatalf("info 级别下应输出 info 日志: %s", out)
	}
}

// LOG_LEVEL=debug 时 Level() 返回 debug，供 GORM 等适配层对齐。
func TestInit_DebugLevel(t *testing.T) {
	t.Setenv("LOG_LEVEL", "debug")
	dir := newLogDir(t)

	Init(dir)
	if got := Level(); got != slog.LevelDebug {
		t.Fatalf("Level() = %v，期望 debug", got)
	}
}

// 日志应同时落到标准错误与按日期命名的文件。
func TestInit_WritesToDailyFile(t *testing.T) {
	t.Setenv("LOG_LEVEL", "info")
	t.Setenv("LOG_FORMAT", "text")
	dir := newLogDir(t)

	Init(dir)
	slog.Info("落盘验证", "answer", 42)

	path := filepath.Join(dir, time.Now().Format("2006-01-02")+".log")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("日志文件未生成: %v", err)
	}
	if !strings.Contains(string(raw), "落盘验证") || !strings.Contains(string(raw), "answer=42") {
		t.Fatalf("日志文件内容不正确:\n%s", raw)
	}
}

// 跨天时应切换到新文件，并把后续日志写进新文件。
func TestDailyWriter_RotatesByDay(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 3, 1, 23, 59, 0, 0, time.UTC)
	clock := func() time.Time { return now }

	w, err := newDailyWriter(dir, clock)
	if err != nil {
		t.Fatalf("构造 writer 失败: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })
	if _, err := w.Write([]byte("第一天\n")); err != nil {
		t.Fatalf("写入失败: %v", err)
	}

	// 跨过零点
	now = now.Add(2 * time.Minute)
	if _, err := w.Write([]byte("第二天\n")); err != nil {
		t.Fatalf("写入失败: %v", err)
	}

	day1, err := os.ReadFile(filepath.Join(dir, "2026-03-01.log"))
	if err != nil {
		t.Fatalf("读取第一天日志失败: %v", err)
	}
	if !strings.Contains(string(day1), "第一天") || strings.Contains(string(day1), "第二天") {
		t.Fatalf("第一天文件内容不正确:\n%s", day1)
	}

	day2, err := os.ReadFile(filepath.Join(dir, "2026-03-02.log"))
	if err != nil {
		t.Fatalf("读取第二天日志失败: %v", err)
	}
	if !strings.Contains(string(day2), "第二天") {
		t.Fatalf("第二天文件内容不正确:\n%s", day2)
	}
}

// 目录不可创建时应返回错误而不是 panic，调用方据此退回标准错误输出。
func TestNewDailyWriter_InvalidDir(t *testing.T) {
	// 用文件当目录，必然创建失败
	file := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("准备文件失败: %v", err)
	}
	if _, err := newDailyWriter(filepath.Join(file, "sub"), time.Now); err == nil {
		t.Fatal("目录不可创建时应返回错误")
	}
}

// newTestLogger 返回写入 buffer 的 textHandler logger。
func newTestLogger(level slog.Level) (*slog.Logger, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	h := newTextHandler(buf, &slog.HandlerOptions{Level: level, AddSource: true})
	return slog.New(h), buf
}

// 输出形状
func TestTextHandler_Shape(t *testing.T) {
	log, buf := newTestLogger(slog.LevelInfo)
	log.InfoContext(context.Background(), "开始投递", "reminder", 12)

	line := strings.TrimSuffix(buf.String(), "\n")
	if !strings.HasPrefix(line, "[INFO] ") {
		t.Fatalf("应以 [INFO] 开头: %q", line)
	}
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

// request_id 要提到消息之前单独成一列
func TestTextHandler_RequestIDColumn(t *testing.T) {
	log, buf := newTestLogger(slog.LevelInfo)
	log.Info("请求返回业务错误", "request_id", "abc123", "status", 400)

	line := buf.String()
	want := " | abc123 | 请求返回业务错误"
	if !strings.Contains(line, want) {
		t.Fatalf("request_id 应位于消息之前（期望包含 %q）:\n%s", want, line)
	}
	if strings.Contains(line, "request_id=abc123") {
		t.Fatalf("request_id 不应重复出现在 kv 中:\n%s", line)
	}
}

// 带空格的值要加引号
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

// error 值直接打印可读文本
func TestTextHandler_ErrorValue(t *testing.T) {
	log, buf := newTestLogger(slog.LevelWarn)
	log.Warn("失败", "error", errors.New("boom"))

	if got := buf.String(); !strings.Contains(got, "error=boom") {
		t.Fatalf("error 值应输出文本:\n%s", got)
	}
}

// Info 级别不带 src；Warn 及以上带
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

// 级别过滤：Info 级别下 Debug 不输出
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

// With 附加的字段要出现在每条日志里
func TestTextHandler_WithAttrs(t *testing.T) {
	log, buf := newTestLogger(slog.LevelInfo)
	log.With("provider", "openai").Info("对话完成", "rounds", 2)

	out := buf.String()
	if !strings.Contains(out, "provider=openai") || !strings.Contains(out, "rounds=2") {
		t.Fatalf("With 字段应出现在日志中:\n%s", out)
	}
}

// 级别标签映射
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

// 时间字段按 2006/01/02 - 15:04:05 渲染
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
