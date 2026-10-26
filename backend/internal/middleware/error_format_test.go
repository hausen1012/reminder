package middleware

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// 统一错误格式的契约：
//
//	{"error": {"code": "...", "message": "...", "field": "..."(可选)}}
//
// 这是整个后端唯一的错误响应形状。本文件把「所有错误出口」逐一断言一遍，
// 防止任何一处回退成旧的 {"code":401,"message":"...","data":null} 包装
//（历史上鉴权中间件就是这么写的，导致前端要兼容两套解析）。

// errorEnvelope 是错误响应的强类型表示。
type errorEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Field   string `json:"field"`
	} `json:"error"`
}

// assertUnifiedError 断言响应体符合统一错误格式，并返回解析结果。
func assertUnifiedError(t *testing.T, w *httptest.ResponseRecorder, wantStatus int, wantCode string) errorEnvelope {
	t.Helper()

	if w.Code != wantStatus {
		t.Fatalf("状态码 = %d，期望 %d，body=%s", w.Code, wantStatus, w.Body.String())
	}

	var env errorEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("响应不是合法 JSON: %v, body=%s", err, w.Body.String())
	}

	if env.Error.Code != wantCode {
		t.Fatalf("error.code = %q，期望 %q（完整响应: %s）", env.Error.Code, wantCode, w.Body.String())
	}
	if env.Error.Message == "" {
		t.Fatalf("error.message 不应为空: %s", w.Body.String())
	}

	// 关键断言：顶层不允许出现旧的 code/message/data 字段
	var top map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &top); err != nil {
		t.Fatalf("无法解析顶层字段: %v", err)
	}
	for _, forbidden := range []string{"code", "message", "data"} {
		if _, exists := top[forbidden]; exists {
			t.Fatalf("顶层不应出现 %q（旧格式残留），body=%s", forbidden, w.Body.String())
		}
	}
	if _, exists := top["error"]; !exists {
		t.Fatalf("顶层应有 error 字段，body=%s", w.Body.String())
	}
	return env
}

// 鉴权中间件：缺少 Authorization 头。
func TestUnifiedError_JWTAuthMissingHeader(t *testing.T) {
	r := gin.New()
	r.Use(ErrorHandler())
	r.GET("/x", JWTAuth("secret"), func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
	assertUnifiedError(t, w, http.StatusUnauthorized, string(CodeUnauthorized))
}

// 鉴权中间件：令牌无效。
func TestUnifiedError_JWTAuthInvalidToken(t *testing.T) {
	r := gin.New()
	r.Use(ErrorHandler())
	r.GET("/x", JWTAuth("secret"), func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer not-a-jwt")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assertUnifiedError(t, w, http.StatusUnauthorized, string(CodeUnauthorized))
}

// 令牌中间件：缺少 X-AUTH。
func TestUnifiedError_TokenAuthMissingHeader(t *testing.T) {
	r := gin.New()
	r.Use(ErrorHandler())
	verify := func(string) (uint, bool) { return 1, true }
	r.GET("/x", TokenAuth(verify, nil, nil), func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
	assertUnifiedError(t, w, http.StatusUnauthorized, string(CodeUnauthorized))
}

// 令牌中间件：令牌无效。
func TestUnifiedError_TokenAuthInvalid(t *testing.T) {
	r := gin.New()
	r.Use(ErrorHandler())
	verify := func(string) (uint, bool) { return 0, false }
	r.GET("/x", TokenAuth(verify, nil, nil), func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("X-AUTH", "bad")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assertUnifiedError(t, w, http.StatusUnauthorized, string(CodeUnauthorized))
}

// 令牌中间件：触发限流。
func TestUnifiedError_TokenAuthRateLimited(t *testing.T) {
	r := gin.New()
	r.Use(ErrorHandler())
	verify := func(string) (uint, bool) { return 1, true }
	rl := NewRateLimiter()
	r.GET("/x", TokenAuth(verify, nil, rl), func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	// 打满 60 次限额
	for i := 0; i < 60; i++ {
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.Header.Set("X-AUTH", "good")
		r.ServeHTTP(httptest.NewRecorder(), req)
	}

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("X-AUTH", "good")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assertUnifiedError(t, w, http.StatusTooManyRequests, string(CodeRateLimited))
}

// 业务 handler 注册的 AppError。
func TestUnifiedError_AppErrorFromHandler(t *testing.T) {
	cases := []struct {
		code   ErrorCode
		status int
	}{
		{CodeValidationFailed, http.StatusBadRequest},
		{CodeUnauthorized, http.StatusUnauthorized},
		{CodeForbidden, http.StatusForbidden},
		{CodeNotFound, http.StatusNotFound},
		{CodeConflict, http.StatusConflict},
		{CodeRateLimited, http.StatusTooManyRequests},
		{CodeInternalError, http.StatusInternalServerError},
	}

	for _, tc := range cases {
		t.Run(string(tc.code), func(t *testing.T) {
			r := gin.New()
			r.Use(ErrorHandler())
			r.GET("/x", func(c *gin.Context) {
				_ = c.Error(NewAppError(tc.code, "测试消息"))
				c.Abort()
			})

			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
			assertUnifiedError(t, w, tc.status, string(tc.code))
		})
	}
}

// 带 field 的 AppError：field 必须原样出现在 error 对象内。
func TestUnifiedError_WithField(t *testing.T) {
	r := gin.New()
	r.Use(ErrorHandler())
	r.GET("/x", func(c *gin.Context) {
		_ = c.Error(NewAppError(CodeValidationFailed, "标题必填").WithField("title"))
		c.Abort()
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
	env := assertUnifiedError(t, w, http.StatusBadRequest, string(CodeValidationFailed))

	if env.Error.Field != "title" {
		t.Fatalf("error.field = %q，期望 title", env.Error.Field)
	}
}

// 未知错误降级为 internal_error，且不泄露内部细节。
func TestUnifiedError_UnknownError(t *testing.T) {
	r := gin.New()
	r.Use(ErrorHandler())
	r.GET("/x", func(c *gin.Context) {
		_ = c.Error(errors.New("数据库连接串密码是 secret"))
		c.Abort()
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
	env := assertUnifiedError(t, w, http.StatusInternalServerError, string(CodeInternalError))

	if env.Error.Message != "服务器内部错误" {
		t.Fatalf("不应泄露内部错误细节，实际 message=%q", env.Error.Message)
	}
}

// panic 恢复后的响应也必须是统一格式。
func TestUnifiedError_PanicRecovered(t *testing.T) {
	r := gin.New()
	r.Use(ErrorHandler())
	r.GET("/x", func(c *gin.Context) { panic("boom") })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
	assertUnifiedError(t, w, http.StatusInternalServerError, string(CodeInternalError))
}

// ErrorBody 是唯一的错误构造入口：直接验证其形状。
func TestErrorBody_Shape(t *testing.T) {
	t.Run("无 field", func(t *testing.T) {
		raw, _ := json.Marshal(ErrorBody(CodeNotFound, "不存在", ""))
		var top map[string]any
		_ = json.Unmarshal(raw, &top)

		if len(top) != 1 {
			t.Fatalf("顶层只应有 error 字段，实际: %s", raw)
		}
		inner := top["error"].(map[string]any)
		if inner["code"] != string(CodeNotFound) || inner["message"] != "不存在" {
			t.Fatalf("error 内部字段不正确: %s", raw)
		}
		if _, exists := inner["field"]; exists {
			t.Fatalf("空 field 应被省略: %s", raw)
		}
	})

	t.Run("有 field", func(t *testing.T) {
		raw, _ := json.Marshal(ErrorBody(CodeValidationFailed, "必填", "title"))
		var top map[string]any
		_ = json.Unmarshal(raw, &top)

		inner := top["error"].(map[string]any)
		if inner["field"] != "title" {
			t.Fatalf("field 应写入: %s", raw)
		}
	})
}
