package middleware

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// captureSlog 把全局 slog 临时重定向到内存 buffer。
//
// slog.SetDefault 是进程级状态，因此用到它的用例不要 t.Parallel。
func captureSlog(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(old) })
	return buf
}

// newLoggingRouter 装配与生产一致的日志链路：RequestID → SlogLogger → ErrorHandler。
func newLoggingRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestID())
	r.Use(SlogLogger())
	r.Use(ErrorHandler())
	return r
}

// 业务错误必须落日志，并带上 code / message / path / request_id。
func TestErrorHandler_LogsAppError(t *testing.T) {
	logs := captureSlog(t)

	r := newLoggingRouter()
	r.GET("/x", func(c *gin.Context) {
		_ = c.Error(NewAppError(CodeNotFound, "提醒不存在").WithField("id"))
		c.Abort()
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))

	out := logs.String()
	for _, want := range []string{
		"msg=请求返回业务错误",
		"error_code=not_found",
		"error_message=提醒不存在",
		"field=id",
		"path=/x",
		"request_id=",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("错误日志缺少 %q:\n%s", want, out)
		}
	}
	// 4xx 属于客户端问题，记为 Warn 而不是 Error
	if !strings.Contains(out, "level=WARN") {
		t.Fatalf("4xx 业务错误应为 WARN 级别:\n%s", out)
	}
	// 响应头与日志里的 request_id 必须一致，否则串不起来
	if id := w.Header().Get(HeaderRequestID); id == "" || !strings.Contains(out, "request_id="+id) {
		t.Fatalf("日志里的 request_id 应与响应头一致（响应头=%q）:\n%s", id, out)
	}
}

// 5xx 的业务错误记为 Error 级别。
func TestErrorHandler_LogsInternalAsError(t *testing.T) {
	logs := captureSlog(t)

	r := newLoggingRouter()
	r.GET("/x", func(c *gin.Context) {
		_ = c.Error(NewAppError(CodeInternalError, "炸了"))
		c.Abort()
	})
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/x", nil))

	if !strings.Contains(logs.String(), "level=ERROR") {
		t.Fatalf("5xx 业务错误应为 ERROR 级别:\n%s", logs.String())
	}
}

// 未知错误必须记录原始错误内容（响应体仍不泄露）。
func TestErrorHandler_LogsUnknownErrorDetail(t *testing.T) {
	logs := captureSlog(t)

	r := newLoggingRouter()
	r.GET("/x", func(c *gin.Context) {
		_ = c.Error(errors.New("sqlite3: database is locked"))
		c.Abort()
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))

	if !strings.Contains(logs.String(), "database is locked") {
		t.Fatalf("日志应包含原始错误详情:\n%s", logs.String())
	}
	if strings.Contains(w.Body.String(), "database is locked") {
		t.Fatalf("响应体不应泄露内部错误:\n%s", w.Body.String())
	}
}

// panic 日志应带堆栈，便于定位。
func TestErrorHandler_LogsPanicWithStack(t *testing.T) {
	logs := captureSlog(t)

	r := newLoggingRouter()
	r.GET("/x", func(c *gin.Context) { panic("boom") })
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/x", nil))

	out := logs.String()
	if !strings.Contains(out, "panic 恢复") || !strings.Contains(out, "stack=") {
		t.Fatalf("panic 日志应带堆栈:\n%s", out)
	}
}

// 请求日志按状态码分级：2xx=INFO、4xx=WARN、5xx=ERROR。
func TestSlogLogger_LevelByStatus(t *testing.T) {
	cases := []struct {
		name   string
		status int
		level  string
		code   ErrorCode
	}{
		{"成功", http.StatusOK, "level=INFO", ""},
		{"客户端错误", http.StatusBadRequest, "level=WARN", CodeValidationFailed},
		{"服务端错误", http.StatusInternalServerError, "level=ERROR", CodeInternalError},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			logs := captureSlog(t)

			r := newLoggingRouter()
			r.GET("/x", func(c *gin.Context) {
				if tc.code != "" {
					_ = c.Error(NewAppError(tc.code, "x"))
					c.Abort()
					return
				}
				c.Status(tc.status)
			})
			r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/x", nil))

			out := logs.String()
			if !strings.Contains(out, "msg=request") {
				t.Fatalf("缺少请求日志:\n%s", out)
			}
			// 取请求日志那一行判断级别（错误日志也会出现在同一个 buffer 里）
			for _, line := range strings.Split(out, "\n") {
				if strings.Contains(line, "msg=request") && !strings.Contains(line, tc.level) {
					t.Fatalf("状态 %d 的请求日志级别不对:\n%s", tc.status, line)
				}
			}
		})
	}
}

// 请求日志应带上身份与路由模板，便于按用户/接口聚合。
func TestSlogLogger_IncludesIdentityAndRoute(t *testing.T) {
	logs := captureSlog(t)

	r := newLoggingRouter()
	r.GET("/api/reminders/:id", func(c *gin.Context) {
		c.Set("user_id", uint(7))
		c.String(http.StatusOK, "ok")
	})

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/reminders/12", nil))

	out := logs.String()
	for _, want := range []string{"route=/api/reminders/:id", "user_id=7", "status=200"} {
		if !strings.Contains(out, want) {
			t.Fatalf("请求日志缺少 %q:\n%s", want, out)
		}
	}
}
