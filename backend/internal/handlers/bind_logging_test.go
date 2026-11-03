package handlers

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/reminder/backend/internal/middleware"
	"github.com/reminder/backend/internal/services"
)

// captureSlog 把全局 slog 临时重定向到内存 buffer。
func captureSlog(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(old) })
	return buf
}

// 请求体解析失败时，对外仍是一句可读提示，但日志里必须能看到底层原因
// （缺字段 / 类型不对 / JSON 语法错），否则「请求体格式错误」等于没记。
func TestAbortBindErr_LogsUnderlyingReason(t *testing.T) {
	logs := captureSlog(t)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.RequestID())
	r.Use(middleware.ErrorHandler())

	h := &ReminderHandler{Svc: services.NewReminderService(newTestDB(t), noopScheduler{}, testTZ(cstZone()), nil, nil)}
	r.POST("/reminders", h.Create)

	// title 是字符串字段，传数字会触发类型错误
	req := newRequest(http.MethodPost, "/reminders", map[string]any{"title": 123})
	w := serve(r, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("状态码 = %d，期望 400，body=%s", w.Code, w.Body.String())
	}
	// 对外不暴露字段级细节
	if body := w.Body.String(); strings.Contains(body, "json") {
		t.Fatalf("响应体不应暴露底层解析细节: %s", body)
	}

	out := logs.String()
	if !strings.Contains(out, "请求体解析失败") {
		t.Fatalf("缺少请求体解析失败日志:\n%s", out)
	}
	// 类型错误的具体原因应出现在日志里
	if !strings.Contains(out, "cannot unmarshal") && !strings.Contains(out, "Unmarshal") {
		t.Fatalf("日志应包含底层错误原因:\n%s", out)
	}
	if !strings.Contains(out, "path=/reminders") {
		t.Fatalf("日志应带请求路径:\n%s", out)
	}
	if !strings.Contains(out, "request_id=") {
		t.Fatalf("日志应带 request_id:\n%s", out)
	}
}

// 空的请求体也应记录原因，而不是无声地只回 400。
func TestAbortBindErr_EmptyBody(t *testing.T) {
	logs := captureSlog(t)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.ErrorHandler())
	h := &ChannelHandler{Svc: services.NewChannelService(newTestDB(t), nil, testTZ(cstZone()))}
	r.POST("/channels", h.Create)

	req := httptest.NewRequest(http.MethodPost, "/channels", nil)
	req.Header.Set("Content-Type", "application/json")
	w := serve(r, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("状态码 = %d，期望 400", w.Code)
	}
	if !strings.Contains(logs.String(), "请求体解析失败") {
		t.Fatalf("空请求体也应记录日志:\n%s", logs.String())
	}
}
