package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// 普通请求应带上全部 CORS 头并继续执行。
func TestCORS_SetsHeaders(t *testing.T) {
	r := newTestRouter()
	r.Use(CORS())
	r.GET("/x", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d", w.Code)
	}
	h := w.Header()
	if h.Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("Allow-Origin = %q", h.Get("Access-Control-Allow-Origin"))
	}
	methods := h.Get("Access-Control-Allow-Methods")
	for _, m := range []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"} {
		if !contains(methods, m) {
			t.Fatalf("Allow-Methods 缺少 %s: %q", m, methods)
		}
	}
	headers := h.Get("Access-Control-Allow-Headers")
	for _, name := range []string{"Content-Type", "Authorization", "X-AUTH"} {
		if !contains(headers, name) {
			t.Fatalf("Allow-Headers 缺少 %s: %q", name, headers)
		}
	}
}

// 预检请求应直接返回 204 且不进入业务 handler。
func TestCORS_PreflightShortCircuits(t *testing.T) {
	r := newTestRouter()
	r.Use(CORS())

	reached := false
	r.OPTIONS("/x", func(c *gin.Context) {
		reached = true
		c.String(http.StatusOK, "should not reach")
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodOptions, "/x", nil))

	if w.Code != http.StatusNoContent {
		t.Fatalf("状态码 = %d，期望 204", w.Code)
	}
	if reached {
		t.Fatal("预检请求不应进入业务 handler")
	}
}

// 请求日志中间件应在响应后记录，且不影响业务响应。
func TestSlogLogger_PassesThrough(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(SlogLogger())
	r.GET("/x", func(c *gin.Context) { c.JSON(http.StatusCreated, gin.H{"ok": true}) })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x?q=1", nil))

	if w.Code != http.StatusCreated {
		t.Fatalf("状态码 = %d，期望 201", w.Code)
	}
	if !contains(w.Body.String(), `"ok":true`) {
		t.Fatalf("响应体被改写: %s", w.Body.String())
	}
}

// 带 query 与 error 的请求也应正常记录（覆盖分支，不 panic）。
func TestSlogLogger_WithQueryAndError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(ErrorHandler())
	r.Use(SlogLogger())
	r.GET("/x", func(c *gin.Context) {
		_ = c.Error(NewAppError(CodeNotFound, "找不到"))
		c.Abort()
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x?q=1&r=2", nil))

	if w.Code != http.StatusNotFound {
		t.Fatalf("状态码 = %d，期望 404", w.Code)
	}
}

// panic 发生在响应已写出之后：不应重复写响应体。
func TestErrorHandler_PanicAfterWriteKeepsBody(t *testing.T) {
	r := newTestRouter()
	r.GET("/x", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"first": true})
		panic("写完之后崩了")
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d，期望保持 200", w.Code)
	}
	body := w.Body.String()
	if !contains(body, `"first":true`) {
		t.Fatalf("原始响应体应保留: %s", body)
	}
	if contains(body, "internal_error") {
		t.Fatalf("不应追加第二个错误体: %s", body)
	}
}
