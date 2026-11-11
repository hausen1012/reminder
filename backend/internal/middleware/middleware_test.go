package middleware

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/reminder/backend/internal/logger"
)

// ==================== auth_test.go ====================

func newTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(ErrorHandler())
	return r
}

// 无 Authorization 头必须拒绝。
func TestJWTAuth_MissingHeader(t *testing.T) {
	r := newTestRouter()
	r.GET("/x", JWTAuth("secret"), func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("状态码 = %d, 期望 401", w.Code)
	}
}

// 非 Bearer 前缀必须拒绝。
func TestJWTAuth_WrongScheme(t *testing.T) {
	r := newTestRouter()
	r.GET("/x", JWTAuth("secret"), func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Basic abc")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("状态码 = %d, 期望 401", w.Code)
	}
}

// 用别的密钥签发的令牌必须拒绝。
func TestJWTAuth_WrongSecret(t *testing.T) {
	r := newTestRouter()
	r.GET("/x", JWTAuth("right-secret"), func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	token := signToken(t, "wrong-secret", time.Now().Add(time.Hour))
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("状态码 = %d, 期望 401", w.Code)
	}
}

// 已过期的令牌必须拒绝。
func TestJWTAuth_Expired(t *testing.T) {
	r := newTestRouter()
	r.GET("/x", JWTAuth("secret"), func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	token := signToken(t, "secret", time.Now().Add(-time.Hour))
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("状态码 = %d, 期望 401", w.Code)
	}
}

// 合法令牌应放行，并把 user_id / username 注入上下文。
func TestJWTAuth_Valid(t *testing.T) {
	r := newTestRouter()
	r.GET("/x", JWTAuth("secret"), func(c *gin.Context) {
		id, _ := c.Get("user_id")
		name, _ := c.Get("username")
		c.JSON(http.StatusOK, gin.H{"user_id": id, "username": name})
	})

	token := signToken(t, "secret", time.Now().Add(time.Hour))
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, 期望 200, body=%s", w.Code, w.Body.String())
	}

	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("响应不是合法 JSON: %v", err)
	}
	if got["username"] != "admin" {
		t.Fatalf("username = %v, 期望 admin", got["username"])
	}
	if got["user_id"] != float64(7) {
		t.Fatalf("user_id = %v, 期望 7", got["user_id"])
	}
}

func signToken(t *testing.T, secret string, exp time.Time) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id":  uint(7),
		"username": "admin",
		"exp":      exp.Unix(),
	})
	s, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("签发令牌失败: %v", err)
	}
	return s
}

// alg=none 攻击：伪造一个无签名令牌，必须被拒绝。
func TestJWTAuth_RejectsNoneAlgorithm(t *testing.T) {
	r := newTestRouter()
	r.GET("/x", JWTAuth("secret"), func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	// 手工拼一个 alg=none 的令牌
	forged := "eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0." +
		"eyJ1c2VyX2lkIjoxLCJ1c2VybmFtZSI6ImhhY2tlciIsImV4cCI6OTk5OTk5OTk5OX0."

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+forged)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("alg=none 令牌应被拒绝，实际 %d, body=%s", w.Code, w.Body.String())
	}
}

// 用非 HMAC 算法（这里用 HS512 以外的合法算法不可签，故用篡改头部测试）：
// 头部声明 RS256 但用 HMAC 签名，必须被拒绝。
func TestJWTAuth_RejectsAlgorithmConfusion(t *testing.T) {
	r := newTestRouter()
	r.GET("/x", JWTAuth("secret"), func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	// 头部 alg=RS256，实际无有效签名
	forged := "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9." +
		"eyJ1c2VyX2lkIjoxLCJleHAiOjk5OTk5OTk5OTl9." +
		"ZmFrZXNpZ25hdHVyZQ"

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+forged)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("算法混淆令牌应被拒绝，实际 %d", w.Code)
	}
}

// 空 Bearer（只有前缀）应被拒绝。
func TestJWTAuth_EmptyBearer(t *testing.T) {
	r := newTestRouter()
	r.GET("/x", JWTAuth("secret"), func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer ")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("空 Bearer 应被拒绝，实际 %d", w.Code)
	}
}

// 格式完全错误的字符串应被拒绝。
func TestJWTAuth_MalformedToken(t *testing.T) {
	r := newTestRouter()
	r.GET("/x", JWTAuth("secret"), func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer not.a.jwt")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("格式错误应被拒绝，实际 %d", w.Code)
	}
}

// claims 缺少 user_id 时视为非法令牌，应拒绝（不能放行为 user_id=0）。
func TestJWTAuth_MissingUserIDClaim(t *testing.T) {
	r := newTestRouter()
	r.GET("/x", JWTAuth("secret"), func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	// 只带 username，不带 user_id
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"username": "ghost",
		"exp":      time.Now().Add(time.Hour).Unix(),
	})
	signed, err := token.SignedString([]byte("secret"))
	if err != nil {
		t.Fatalf("签发令牌失败: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+signed)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("缺少 user_id 的令牌应被拒绝，实际 %d, body=%s", w.Code, w.Body.String())
	}
}

// user_id 类型错误（字符串）时解析失败，应拒绝而非当作 0 放行。
func TestJWTAuth_WrongClaimType(t *testing.T) {
	r := newTestRouter()
	r.GET("/x", JWTAuth("secret"), func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id":  "not-a-number",
		"username": "admin",
		"exp":      time.Now().Add(time.Hour).Unix(),
	})
	signed, err := token.SignedString([]byte("secret"))
	if err != nil {
		t.Fatalf("签发令牌失败: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+signed)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("user_id 类型错误的令牌应被拒绝，实际 %d, body=%s", w.Code, w.Body.String())
	}
}

// ==================== token_test.go ====================

// 每分钟限额内应放行，超过后返回 429。
func TestTokenAuth_RateLimited(t *testing.T) {
	r := newTestRouter()
	verify := func(plain string) (uint, bool) { return 1, plain == "good" }
	r.GET("/x", TokenAuth(verify, nil, nil), func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	// 前 60 次应全部通过
	for i := 0; i < 60; i++ {
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.Header.Set("X-AUTH", "good")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("第 %d 次请求状态码 = %d, 期望 200", i+1, w.Code)
		}
	}

	// 第 61 次触发限流
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("X-AUTH", "good")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("状态码 = %d, 期望 429", w.Code)
	}
}

// 缺少 X-AUTH 头应返回 401。
func TestTokenAuth_MissingHeader(t *testing.T) {
	r := newTestRouter()
	verify := func(plain string) (uint, bool) { return 1, true }
	r.GET("/x", TokenAuth(verify, nil, nil), func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("状态码 = %d, 期望 401", w.Code)
	}
}

// 令牌无效应返回 401。
func TestTokenAuth_InvalidToken(t *testing.T) {
	r := newTestRouter()
	verify := func(plain string) (uint, bool) { return 0, false }
	r.GET("/x", TokenAuth(verify, nil, nil), func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("X-AUTH", "bad")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("状态码 = %d, 期望 401", w.Code)
	}
}

// 通过的请求应注入 token_id 并调用 toucher。
func TestTokenAuth_InjectsTokenID(t *testing.T) {
	r := newTestRouter()
	verify := func(plain string) (uint, bool) { return 42, true }

	var touched uint
	r.GET("/x", TokenAuth(verify, func(id uint) { touched = id }, nil), func(c *gin.Context) {
		id, _ := c.Get("token_id")
		c.JSON(http.StatusOK, gin.H{"token_id": id})
	})

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("X-AUTH", "good")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, 期望 200", w.Code)
	}
	if touched != 42 {
		t.Fatalf("toucher 收到 %d, 期望 42", touched)
	}
}

// ==================== cors_test.go ====================

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

// ==================== request_id_test.go ====================

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

// ==================== bodylimit_test.go ====================

// newBodyLimitEngine 构造带 BodyLimit 与统一错误处理的最小引擎。
func newBodyLimitEngine(maxBytes int64) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(BodyLimit(maxBytes))
	r.Use(ErrorHandler())
	r.POST("/echo", func(c *gin.Context) {
		var in map[string]any
		if err := c.ShouldBindJSON(&in); err != nil {
			if IsBodyTooLarge(err) {
				c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge,
					ErrorBody(CodePayloadTooLarge, "请求体超过大小限制", ""))
				return
			}
			c.Data(http.StatusBadRequest, "text/plain", []byte(err.Error()))
			return
		}
		c.Status(http.StatusOK)
	})
	return r
}

// 声明了 Content-Length 且超限时应在读取前直接拒绝。
func TestBodyLimit_RejectsDeclaredOversize(t *testing.T) {
	r := newBodyLimitEngine(16)

	body := strings.NewReader(`{"title":"` + strings.Repeat("a", 32) + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/echo", body)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("状态码 = %d，期望 413，body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), string(CodePayloadTooLarge)) {
		t.Fatalf("错误码应为 %q，实际 body=%s", CodePayloadTooLarge, w.Body.String())
	}
}

// 未声明长度（chunked）时由 MaxBytesReader 在读超限时报错。
func TestBodyLimit_RejectsChunkedOversize(t *testing.T) {
	r := newBodyLimitEngine(16)

	req := httptest.NewRequest(http.MethodPost, "/echo", io.NopCloser(strings.NewReader(
		`{"title":"`+strings.Repeat("a", 64)+`"}`,
	)))
	req.ContentLength = -1
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("状态码 = %d，期望 413，body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), string(CodePayloadTooLarge)) {
		t.Fatalf("错误码应为 %q，实际 body=%s", CodePayloadTooLarge, w.Body.String())
	}
}

// 正常大小的请求必须不受影响。
func TestBodyLimit_AllowsNormalRequest(t *testing.T) {
	r := newBodyLimitEngine(1024)
	req := httptest.NewRequest(http.MethodPost, "/echo", strings.NewReader(`{"title":"hi"}`))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d，期望 200，body=%s", w.Code, w.Body.String())
	}
}

// maxBytes <= 0 时应完全不限制。
func TestBodyLimit_ZeroDisables(t *testing.T) {
	r := newBodyLimitEngine(0)
	req := httptest.NewRequest(http.MethodPost, "/echo", strings.NewReader(`{"title":"`+strings.Repeat("a", 4096)+`"}`))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d，期望 200", w.Code)
	}
}

// ==================== ratelimiter_test.go ====================

// 过期槽位应被惰性清理，避免长期运行时内存只增不减。
func TestRateLimiter_CleansUpExpiredSlots(t *testing.T) {
	rl := NewRateLimiter()

	// 塞入 10 个已过期的槽位
	rl.mu.Lock()
	for i := uint(1); i <= 10; i++ {
		rl.slots[i] = &rateSlot{count: 1, resetAt: time.Now().Add(-time.Minute)}
	}
	rl.mu.Unlock()

	// 触发一次清理（Allow 每 256 次调用清理一次）
	for i := 0; i < 256; i++ {
		rl.Allow(999, 60)
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()
	if len(rl.slots) != 1 {
		t.Fatalf("槽位数 = %d, 期望只剩 1 个活跃槽位", len(rl.slots))
	}
	if _, ok := rl.slots[999]; !ok {
		t.Fatal("活跃槽位不应被清理")
	}
}

// 登录限流：连续失败达到阈值后应锁定，成功则清零。
func TestLoginLimiter_LocksAfterFailures(t *testing.T) {
	l := NewLoginLimiter(3, time.Minute)
	key := "admin|1.2.3.4"

	for i := 0; i < 2; i++ {
		if !l.Allow(key) {
			t.Fatalf("第 %d 次失败前不应锁定", i+1)
		}
		l.RecordFailure(key)
	}
	if !l.Allow(key) {
		t.Fatal("达到阈值前仍应允许尝试")
	}
	l.RecordFailure(key)
	if l.Allow(key) {
		t.Fatal("连续失败 3 次后应被锁定")
	}
}

// 成功登录应清空该键的失败计数。
func TestLoginLimiter_SuccessResets(t *testing.T) {
	l := NewLoginLimiter(2, time.Minute)
	key := "admin|1.2.3.4"

	l.RecordFailure(key)
	l.RecordSuccess(key)
	if !l.Allow(key) {
		t.Fatal("成功登录后不应仍处于锁定状态")
	}
	l.RecordFailure(key)
	if !l.Allow(key) {
		t.Fatal("计数应已清零")
	}
}

// 锁定期结束后应自动解锁并清零。
func TestLoginLimiter_UnlocksAfterLockout(t *testing.T) {
	l := NewLoginLimiter(1, 10*time.Millisecond)
	key := "admin|1.2.3.4"

	l.RecordFailure(key)
	if l.Allow(key) {
		t.Fatal("应立即锁定")
	}
	time.Sleep(20 * time.Millisecond)
	if !l.Allow(key) {
		t.Fatal("锁定期结束后应解锁")
	}
}

// maxAttempts <= 0 表示关闭限流。
func TestLoginLimiter_DisabledWhenZero(t *testing.T) {
	l := NewLoginLimiter(0, time.Minute)
	key := "admin|1.2.3.4"
	for i := 0; i < 100; i++ {
		l.RecordFailure(key)
	}
	if !l.Allow(key) {
		t.Fatal("关闭限流后不应锁定")
	}
}

// 过期槽位应被惰性清理。
func TestLoginLimiter_CleansUpIdleSlots(t *testing.T) {
	l := NewLoginLimiter(3, time.Minute)

	l.mu.Lock()
	for i := 0; i < 300; i++ {
		l.slots[string(rune('a'+i%26))+itoaTest(i)] = &loginSlot{
			failures: 1,
			lastSeen: time.Now().Add(-2 * time.Hour),
		}
	}
	l.mu.Unlock()

	for i := 0; i < 256; i++ {
		l.Allow("fresh|1.2.3.4")
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	for k, slot := range l.slots {
		if time.Since(slot.lastSeen) > time.Hour && k != "fresh|1.2.3.4" {
			t.Fatalf("过期槽位 %q 未被清理", k)
		}
	}
}

func itoaTest(v int) string {
	if v == 0 {
		return "0"
	}
	var buf [8]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}

// 计数键应为「小写用户名 + IP」，避免改大小写绕过。
func TestLoginKey_NormalizesUsername(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	var got string
	r.POST("/login", func(c *gin.Context) {
		got = LoginKey(c, " Admin ")
		c.Status(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodPost, "/login", nil)
	req.RemoteAddr = "203.0.113.9:1234"
	r.ServeHTTP(httptest.NewRecorder(), req)

	if !strings.HasPrefix(got, "admin|") {
		t.Fatalf("计数键 = %q，期望以 admin| 开头", got)
	}
}

// ==================== logging_test.go ====================

// captureSlog 把全局 slog 临时重定向到内存 buffer。
//
// slog.SetDefault 是进程级状态，因此用到它的用例不要 t.Parallel。
func captureSlog(t *testing.T) *bytes.Buffer {
	t.Helper()
	return captureSlogAt(t, slog.LevelDebug)
}

// captureSlogAt 用指定级别捕获日志。
func captureSlogAt(t *testing.T, level slog.Level) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: level})))
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

// 请求日志按状态码分级：2xx=DEBUG、4xx=WARN、5xx=ERROR。
//
// 2xx 降到 Debug 是有意的：面板每 10 秒轮询一次监控接口，
// Info 级别下这些日志会把真正的问题淹没。
func TestSlogLogger_LevelByStatus(t *testing.T) {
	cases := []struct {
		name   string
		status int
		level  string
		code   ErrorCode
	}{
		{"成功", http.StatusOK, "level=DEBUG", ""},
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

// Info 级别下成功的访问日志应当完全不输出。
//
// 这是「日志不要全是轮询请求」的核心保证：默认配置下只有异常与慢请求可见。
func TestSlogLogger_SuccessSilentAtInfo(t *testing.T) {
	logs := captureSlogAt(t, slog.LevelInfo)

	r := newLoggingRouter()
	r.GET("/api/scheduler/status", func(c *gin.Context) { c.Status(http.StatusOK) })
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/scheduler/status", nil))

	if out := logs.String(); strings.Contains(out, "msg=request") {
		t.Fatalf("info 级别下不应输出成功请求的访问日志:\n%s", out)
	}
}

// 成功但慢的请求要提到 Warn：慢查询与外部超时不能因为没有非 2xx 状态码而被漏掉。
func TestSlogLogger_SlowSuccessIsWarn(t *testing.T) {
	logs := captureSlogAt(t, slog.LevelInfo)

	r := newLoggingRouter()
	r.GET("/slow", func(c *gin.Context) {
		time.Sleep(slowRequestThreshold + 10*time.Millisecond)
		c.Status(http.StatusOK)
	})
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/slow", nil))

	out := logs.String()
	if !strings.Contains(out, "msg=request") || !strings.Contains(out, "level=WARN") {
		t.Fatalf("超过 1 秒的成功请求应为 WARN:\n%s", out)
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

// RequestID 必须把 id 写进请求 ctx，业务层才能用 logger.FromContext 自动带上。
func TestRequestID_InjectsIntoContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestID())

	var fromCtx string
	r.GET("/x", func(c *gin.Context) {
		fromCtx = logger.RequestIDFromContext(c.Request.Context())
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))

	if fromCtx == "" || fromCtx != w.Header().Get(HeaderRequestID) {
		t.Fatalf("ctx 里的 request_id = %q，响应头 = %q，两者应一致且非空",
			fromCtx, w.Header().Get(HeaderRequestID))
	}
}

// 业务层用 logger.FromContext 打日志时，不需要手写 request_id 也能和访问日志对上。
func TestLoggerFromContext_CarriesRequestID(t *testing.T) {
	logs := captureSlog(t)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestID())
	r.GET("/x", func(c *gin.Context) {
		logger.FromContext(c.Request.Context()).Warn("业务日志", "reminder", 7)
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))

	out := logs.String()
	id := w.Header().Get(HeaderRequestID)
	if !strings.Contains(out, "业务日志") || !strings.Contains(out, "request_id="+id) {
		t.Fatalf("业务日志应自动带 request_id=%s:\n%s", id, out)
	}
	if !strings.Contains(out, "reminder=7") {
		t.Fatalf("业务日志缺少自定义字段:\n%s", out)
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
	if !strings.Contains(w.Body.String(), `"ok":true`) {
		t.Fatalf("响应体被改写: %s", w.Body.String())
	}
}

// 带 query 与 error 的请求也应正常记录（覆盖分支，不 panic）。
// query 里的取值属于可能的凭据，只记参数名。
func TestSlogLogger_WithQueryAndError(t *testing.T) {
	logs := captureSlogAt(t, slog.LevelDebug)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(ErrorHandler())
	r.Use(SlogLogger())
	r.GET("/x", func(c *gin.Context) {
		_ = c.Error(NewAppError(CodeNotFound, "找不到"))
		c.Abort()
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x?q=1&token=SECRET", nil))

	if w.Code != http.StatusNotFound {
		t.Fatalf("状态码 = %d，期望 404", w.Code)
	}
	out := logs.String()
	if strings.Contains(out, "SECRET") {
		t.Fatalf("访问日志不应包含 query 取值:\n%s", out)
	}
	if !strings.Contains(out, "token=%2A%2A%2A") {
		t.Fatalf("query 参数名应保留并脱敏取值:\n%s", out)
	}
}

// ==================== errors_test.go ====================

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
