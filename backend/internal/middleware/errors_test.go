package middleware

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// AppError.httpStatus 应根据 ErrorCode 推断正确的 HTTP 状态码。
func TestAppError_HTTPStatusMapping(t *testing.T) {
	cases := []struct {
		code ErrorCode
		want int
	}{
		{CodeUnauthorized, http.StatusUnauthorized},
		{CodeForbidden, http.StatusForbidden},
		{CodeNotFound, http.StatusNotFound},
		{CodeValidationFailed, http.StatusBadRequest},
		{CodeConflict, http.StatusConflict},
		{CodeRateLimited, http.StatusTooManyRequests},
		{CodeInternalError, http.StatusInternalServerError},
		{ErrorCode("unknown_code"), http.StatusInternalServerError},
	}

	for _, tc := range cases {
		t.Run(string(tc.code), func(t *testing.T) {
			if got := NewAppError(tc.code, "msg").httpStatus(); got != tc.want {
				t.Fatalf("code=%s 得到 %d，期望 %d", tc.code, got, tc.want)
			}
		})
	}
}

// WithStatus 应覆盖按 code 推断的默认状态码。
func TestAppError_WithStatusOverrides(t *testing.T) {
	e := NewAppError(CodeValidationFailed, "msg").WithStatus(http.StatusTeapot)
	if got := e.httpStatus(); got != http.StatusTeapot {
		t.Fatalf("得到 %d，期望 418", got)
	}
}

// Error() 应包含 code / message，带 field 时额外包含字段名。
func TestAppError_ErrorMessage(t *testing.T) {
	plain := NewAppError(CodeNotFound, "不存在")
	if got := plain.Error(); got != "not_found: 不存在" {
		t.Fatalf("Error() = %q", got)
	}

	withField := NewAppError(CodeValidationFailed, "必填").WithField("title")
	want := "validation_failed: 必填 (field=title)"
	if got := withField.Error(); got != want {
		t.Fatalf("Error() = %q, 期望 %q", got, want)
	}
}

// handler 通过 c.Error 注册 AppError 时，应渲染为 {error:{code,message}}。
func TestErrorHandler_RendersAppError(t *testing.T) {
	r := newTestRouter()
	r.GET("/x", func(c *gin.Context) {
		_ = c.Error(NewAppError(CodeNotFound, "提醒不存在"))
		c.Abort()
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))

	if w.Code != http.StatusNotFound {
		t.Fatalf("状态码 = %d，期望 404", w.Code)
	}
	if got := w.Body.String(); !contains(got, `"code":"not_found"`) || !contains(got, `"message":"提醒不存在"`) {
		t.Fatalf("错误体不符合契约: %s", got)
	}
}

// 带 field 的 AppError 应在响应体里带上 field。
func TestErrorHandler_RendersField(t *testing.T) {
	r := newTestRouter()
	r.GET("/x", func(c *gin.Context) {
		_ = c.Error(NewAppError(CodeValidationFailed, "标题必填").WithField("title"))
		c.Abort()
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("状态码 = %d，期望 400", w.Code)
	}
	if got := w.Body.String(); !contains(got, `"field":"title"`) {
		t.Fatalf("应包含 field 字段: %s", got)
	}
}

// 非 AppError 的普通错误应统一降级为 500，且不泄露内部细节。
func TestErrorHandler_UnknownErrorBecomes500(t *testing.T) {
	r := newTestRouter()
	r.GET("/x", func(c *gin.Context) {
		_ = c.Error(errors.New("内部数据库炸了"))
		c.Abort()
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("状态码 = %d，期望 500", w.Code)
	}
	body := w.Body.String()
	if !contains(body, `"code":"internal_error"`) {
		t.Fatalf("应返回 internal_error: %s", body)
	}
	// 内部错误细节不应暴露给客户端
	if contains(body, "数据库") {
		t.Fatalf("不应泄露内部错误细节: %s", body)
	}
}

// panic 应被 recover 并返回 500，且进程不崩。
func TestErrorHandler_RecoversPanic(t *testing.T) {
	r := newTestRouter()
	r.GET("/x", func(c *gin.Context) {
		panic("boom")
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("状态码 = %d，期望 500", w.Code)
	}
	if !contains(w.Body.String(), `"code":"internal_error"`) {
		t.Fatalf("应返回 internal_error: %s", w.Body.String())
	}
}

// 多个 error 时只渲染第一个（避免重复写响应头）。
func TestErrorHandler_OnlyFirstErrorRendered(t *testing.T) {
	r := newTestRouter()
	r.GET("/x", func(c *gin.Context) {
		_ = c.Error(NewAppError(CodeValidationFailed, "第一个"))
		_ = c.Error(NewAppError(CodeConflict, "第二个"))
		c.Abort()
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("状态码 = %d，期望 400（取第一个）", w.Code)
	}
	body := w.Body.String()
	if !contains(body, "第一个") {
		t.Fatalf("应渲染第一个错误: %s", body)
	}
	if contains(body, "第二个") {
		t.Fatalf("不应渲染第二个错误: %s", body)
	}
}

// 无 error 时不应改动响应。
func TestErrorHandler_NoErrorPassthrough(t *testing.T) {
	r := newTestRouter()
	r.GET("/x", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d，期望 200", w.Code)
	}
	if !contains(w.Body.String(), `"ok":true`) {
		t.Fatalf("响应体被改写: %s", w.Body.String())
	}
}

// 已写过响应再注册 error 时，不应产生第二个响应体。
func TestErrorHandler_DoesNotDoubleWrite(t *testing.T) {
	r := newTestRouter()
	r.GET("/x", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
		_ = c.Error(NewAppError(CodeInternalError, "迟到的错误"))
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))

	// 已经写过 body，状态码保持 200
	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d，期望保持 200", w.Code)
	}
}

// contains 是 strings.Contains 的简写。
func contains(s, sub string) bool {
	return len(sub) == 0 || indexOf(s, sub) >= 0
}

// indexOf 返回子串首次出现位置，未找到返回 -1。
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

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

	// 顶层结构必须只有 error 一个键
	var top map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &top); err != nil {
		t.Fatalf("无法解析顶层字段: %v", err)
	}
	if _, exists := top["error"]; !exists {
		t.Fatalf("顶层应有 error 字段，body=%s", w.Body.String())
	}
	if len(top) != 1 {
		t.Fatalf("顶层只应有 error 字段，实际 %d 个，body=%s", len(top), w.Body.String())
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
