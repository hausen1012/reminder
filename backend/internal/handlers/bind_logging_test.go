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
//
// 现在底层原因挂在 AppError.Cause 上，由 ErrorHandler 统一输出：
// 一次失败只留一条日志，不会出现「解析失败 + 业务错误」两条重复记录。
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
	if !strings.Contains(out, "请求返回业务错误") {
		t.Fatalf("缺少业务错误日志:\n%s", out)
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
	// 同一次失败只记一条：底层的 bind 错误不再单独打点
	if n := strings.Count(out, "level=WARN"); n != 1 {
		t.Fatalf("请求体解析失败应只产生一条 WARN 日志，实际 %d 条:\n%s", n, out)
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
	out := logs.String()
	if !strings.Contains(out, "请求返回业务错误") || !strings.Contains(out, "error=") {
		t.Fatalf("空请求体也应记录底层原因:\n%s", out)
	}
}
