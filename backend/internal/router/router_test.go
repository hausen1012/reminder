package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/reminder/backend/internal/middleware"
)

// newNotFoundTestEngine 构造一个只装了错误中间件与 404 兜底的最小路由。
func newNotFoundTestEngine() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.ErrorHandler())
	r.GET("/api/health", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	registerNotFound(r)
	return r
}

// 未注册的 /api 路径必须返回统一错误体，而不是 gin 默认的纯文本。
//
// 前端 extractApiError 只认 {"error":{"message":...}}，纯文本会让它退化成
// "Request failed with status code 404"。
func TestNoRoute_API_UsesUnifiedErrorBody(t *testing.T) {
	r := newNotFoundTestEngine()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/not-exist", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("状态码 = %d，期望 404", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct == "" || !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("Content-Type = %q，期望 application/json", ct)
	}

	var resp struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("响应不是合法 JSON: %v, body=%s", err, w.Body.String())
	}
	if resp.Error.Code != string(middleware.CodeNotFound) {
		t.Fatalf("error.code = %q，期望 %q", resp.Error.Code, middleware.CodeNotFound)
	}
	if resp.Error.Message == "" {
		t.Fatal("error.message 不应为空")
	}
}

// 非 /api 路径保持原样（静态资源由前端中间件处理，不做 JSON 包装）。
func TestNoRoute_NonAPI_NotJSON(t *testing.T) {
	r := newNotFoundTestEngine()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/some/static/path.js", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("状态码 = %d，期望 404", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); strings.HasPrefix(ct, "application/json") {
		t.Fatalf("非 /api 路径不应返回 JSON 错误体，Content-Type = %q", ct)
	}
}
