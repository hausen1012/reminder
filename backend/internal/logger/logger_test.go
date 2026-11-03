package logger

import (
	"bytes"
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
