package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/reminder/backend/internal/config"
	"github.com/reminder/backend/internal/middleware"
)

// newMiddlewareOrderEngine 用真实的中间件装配构造最小引擎。
//
// 只挂一个 POST 路由：它的 body 会被读取，从而触发 BodyLimit 的 MaxBytesReader。
func newMiddlewareOrderEngine(t *testing.T, maxBodyMB int64) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	applyMiddleware(r, &config.Config{MaxBodyMB: maxBodyMB})
	r.POST("/api/echo", func(c *gin.Context) {
		var in map[string]any
		if err := c.ShouldBindJSON(&in); err != nil {
			// 与真实 handler 一致：超限走 413 + 统一错误体
			if middleware.IsBodyTooLarge(err) {
				c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge,
					map[string]any{"error": map[string]any{"code": "payload_too_large", "message": "请求体超过大小限制"}})
				return
			}
			c.Data(http.StatusBadRequest, "text/plain", []byte(err.Error()))
			return
		}
		c.Status(http.StatusOK)
	})
	return r
}

// 已声明长度超限：BodyLimit 在 CORS 之后执行，413 必须带跨域头。
//
// 这条用例防的是中间件顺序回归：CORS 若排在 BodyLimit 之后，
// AbortWithStatusJSON 会跳过它，浏览器把 413 当成不可读的跨域失败，
// 前端只能显示「网络错误」而不是「请求过大」。
func TestMiddlewareOrder_DeclaredOversizeKeepsCORS(t *testing.T) {
	r := newMiddlewareOrderEngine(t, 1) // 1 MB

	body := strings.NewReader(`{"title":"` + strings.Repeat("a", 2<<20) + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/echo", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://example.test")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("状态码 = %d，期望 413", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("413 响应缺少 CORS 头：Access-Control-Allow-Origin = %q", got)
	}
}

// 未声明长度（chunked）超限：同样要带跨域头。
func TestMiddlewareOrder_ChunkedOversizeKeepsCORS(t *testing.T) {
	r := newMiddlewareOrderEngine(t, 1)

	req := httptest.NewRequest(http.MethodPost, "/api/echo",
		strings.NewReader(`{"title":"`+strings.Repeat("a", 2<<20)+`"}`))
	req.ContentLength = -1
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://example.test")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("状态码 = %d，期望 413", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("413 响应缺少 CORS 头：Access-Control-Allow-Origin = %q", got)
	}
}

// 业务错误（4xx，由 ErrorHandler 渲染）也应带跨域头。
func TestMiddlewareOrder_BusinessErrorKeepsCORS(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	applyMiddleware(r, &config.Config{MaxBodyMB: 1})
	r.GET("/api/boom", func(c *gin.Context) {
		c.AbortWithStatusJSON(http.StatusBadRequest,
			map[string]any{"error": map[string]any{"code": "validation_failed", "message": "x"}})
	})

	req := httptest.NewRequest(http.MethodGet, "/api/boom", nil)
	req.Header.Set("Origin", "http://example.test")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("业务错误响应缺少 CORS 头：%q", got)
	}
}

// 预检请求仍应被 CORS 自己直接应答，且不进入访问日志之外的处理。
func TestMiddlewareOrder_PreflightAnswered(t *testing.T) {
	r := newMiddlewareOrderEngine(t, 1)
	req := httptest.NewRequest(http.MethodOptions, "/api/echo", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("预检状态码 = %d，期望 204", w.Code)
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatal("预检响应缺少 CORS 头")
	}
}
