package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/reminder/backend/internal/config"
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

// 未注册的 /api 路径必须返回统一 JSON 错误体。
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

func TestInitGinMode(t *testing.T) {
	origMode := gin.Mode()
	origEnv, hadEnv := os.LookupEnv("GIN_MODE")
	t.Cleanup(func() {
		gin.SetMode(origMode)
		if hadEnv {
			_ = os.Setenv("GIN_MODE", origEnv)
		} else {
			_ = os.Unsetenv("GIN_MODE")
		}
	})

	t.Run("未设置GIN_MODE且非TestMode时默认切换为ReleaseMode", func(t *testing.T) {
		_ = os.Unsetenv("GIN_MODE")
		gin.SetMode(gin.DebugMode)
		initGinMode()
		if gin.Mode() != gin.ReleaseMode {
			t.Fatalf("gin.Mode() = %s, 期望 %s", gin.Mode(), gin.ReleaseMode)
		}
	})

	t.Run("已显式配置GIN_MODE环境变量时保持外部设置", func(t *testing.T) {
		_ = os.Setenv("GIN_MODE", "debug")
		gin.SetMode(gin.DebugMode)
		initGinMode()
		if gin.Mode() != gin.DebugMode {
			t.Fatalf("gin.Mode() = %s, 期望 %s", gin.Mode(), gin.DebugMode)
		}
	})

	t.Run("单测环境下已设置为TestMode时予以保留", func(t *testing.T) {
		_ = os.Unsetenv("GIN_MODE")
		gin.SetMode(gin.TestMode)
		initGinMode()
		if gin.Mode() != gin.TestMode {
			t.Fatalf("gin.Mode() = %s, 期望 %s", gin.Mode(), gin.TestMode)
		}
	})
}

// newMiddlewareOrderEngine 用真实的中间件装配构造最小引擎。
func newMiddlewareOrderEngine(t *testing.T, maxBodyMB int64) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	applyMiddleware(r, &config.Config{MaxBodyMB: maxBodyMB})
	r.POST("/api/echo", func(c *gin.Context) {
		var in map[string]any
		if err := c.ShouldBindJSON(&in); err != nil {
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
func TestMiddlewareOrder_DeclaredOversizeKeepsCORS(t *testing.T) {
	r := newMiddlewareOrderEngine(t, 1)

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

// 预检请求仍应被 CORS 自己直接应答。
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

// 反证：把 CORS 挪到 BodyLimit 之后，413 确实会丢掉跨域头。
func TestMiddlewareOrder_WrongOrderDemonstrablyDropsCORS(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.RequestID())
	r.Use(middleware.SlogLogger())
	r.Use(middleware.BodyLimit(1 << 20))
	r.Use(middleware.CORS())
	r.POST("/api/echo", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodPost, "/api/echo",
		strings.NewReader(strings.Repeat("a", 2<<20)))
	req.ContentLength = 2 << 20
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("状态码 = %d，期望 413", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("错误顺序下本应缺少 CORS 头，实际得到 %q", got)
	}
}
