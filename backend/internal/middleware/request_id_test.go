package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// 未带请求 ID 时应自动生成，并写回响应头。
func TestRequestID_GeneratesWhenMissing(t *testing.T) {
	r := newTestRouter()
	r.Use(RequestID())
	r.GET("/x", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))

	id := w.Header().Get(HeaderRequestID)
	if id == "" {
		t.Fatal("响应头应带 X-Request-ID")
	}
	if len(id) != 16 {
		t.Fatalf("自动生成的请求 ID 长度 = %d，期望 16（%q）", len(id), id)
	}
}

// 客户端自带的合法请求 ID 应原样沿用，便于跨服务追踪。
func TestRequestID_ReusesClientID(t *testing.T) {
	r := newTestRouter()
	r.Use(RequestID())

	var got string
	r.GET("/x", func(c *gin.Context) {
		got = RequestIDOf(c)
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set(HeaderRequestID, "gateway-abc-123")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if got != "gateway-abc-123" {
		t.Fatalf("上下文里的请求 ID = %q，期望沿用客户端值", got)
	}
	if w.Header().Get(HeaderRequestID) != "gateway-abc-123" {
		t.Fatalf("响应头 = %q，期望沿用客户端值", w.Header().Get(HeaderRequestID))
	}
}

// 非法请求 ID（超长、带换行）必须被替换，避免写进日志/响应头造成注入。
func TestRequestID_RejectsUnsafeValues(t *testing.T) {
	cases := map[string]string{
		"带换行": "abc\r\nX-Evil: 1",
		"超长":  "abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklmnopqrstuvwxyz0123456789abc",
	}

	for name, value := range cases {
		t.Run(name, func(t *testing.T) {
			r := newTestRouter()
			r.Use(RequestID())

			var got string
			r.GET("/x", func(c *gin.Context) {
				got = RequestIDOf(c)
				c.String(http.StatusOK, "ok")
			})

			req := httptest.NewRequest(http.MethodGet, "/x", nil)
			req.Header.Set(HeaderRequestID, value)
			r.ServeHTTP(httptest.NewRecorder(), req)

			if got == value {
				t.Fatalf("非法请求 ID %q 不应被沿用", value)
			}
			if got == "" {
				t.Fatal("应替换为自动生成的请求 ID")
			}
		})
	}
}

// 未装配中间件时 RequestIDOf 返回空串，不 panic（大量测试路由没有装它）。
func TestRequestIDOf_WithoutMiddleware(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	if got := RequestIDOf(c); got != "" {
		t.Fatalf("未装配中间件时应返回空串，实际 %q", got)
	}
	if got := RequestIDOf(nil); got != "" {
		t.Fatalf("nil context 应返回空串，实际 %q", got)
	}
}
