package handlers

import (
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/reminder/backend/internal/middleware"
	"github.com/reminder/backend/internal/services"
)

// newLoginLimiterEngine 构造带登录限流与统一错误处理的引擎。
func newLoginLimiterEngine(t *testing.T, limiter *middleware.LoginLimiter) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := newTestDB(t)
	seedUser(t, db)

	h := &AuthHandler{JWTSecret: testJWTSecret, UserSvc: services.NewUserService(db), Limiter: limiter}
	r := gin.New()
	r.Use(middleware.BodyLimit(64))
	r.Use(middleware.ErrorHandler())
	r.POST("/login", h.Login)
	return r
}

// 连续失败达到阈值后，正确的密码也应被拒绝（429）。
//
// 登录接口不需要凭据，不设上限就等于允许无限撞库。
func TestLogin_LockedOutAfterRepeatedFailures(t *testing.T) {
	r := newLoginLimiterEngine(t, middleware.NewLoginLimiter(3, 0))

	for i := 0; i < 3; i++ {
		w := doJSON(r, http.MethodPost, "/login", map[string]string{"username": "admin", "password": "wrong"})
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("第 %d 次失败状态码 = %d，期望 401", i+1, w.Code)
		}
	}

	// 即使密码正确也不放行
	w := doJSON(r, http.MethodPost, "/login", map[string]string{"username": "admin", "password": "admin123"})
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("锁定后状态码 = %d，期望 429，body=%s", w.Code, w.Body.String())
	}
	if !containsSub(w.Body.String(), string(middleware.CodeRateLimited)) {
		t.Fatalf("错误码应为 rate_limited，body=%s", w.Body.String())
	}
	if w.Header().Get("Retry-After") == "" {
		t.Fatal("限流响应应带 Retry-After")
	}
}

// Retry-After 必须反映实际锁定时长，而不是写死的 60 秒。
//
// 锁 10 分钟时若仍返回 60，客户端会在一分钟后重试、再吃一次 429，
// 这类「配置改了但响应没跟着改」的脱节正是这条用例要防的。
func TestLogin_RetryAfterMatchesConfiguredLockout(t *testing.T) {
	const lockout = 10 * time.Minute
	r := newLoginLimiterEngine(t, middleware.NewLoginLimiter(1, lockout))

	// 第一次失败即达到阈值
	doJSON(r, http.MethodPost, "/login", map[string]string{"username": "admin", "password": "wrong"})
	w := doJSON(r, http.MethodPost, "/login", map[string]string{"username": "admin", "password": "admin123"})
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("状态码 = %d，期望 429", w.Code)
	}

	got := w.Header().Get("Retry-After")
	secs, err := strconv.Atoi(got)
	if err != nil {
		t.Fatalf("Retry-After = %q，不是整数秒", got)
	}
	// 请求往返会消耗几十毫秒，允许 5 秒误差；关键是量级与配置一致，不能是 60
	if secs < 590 || secs > 600 {
		t.Fatalf("Retry-After = %d 秒，期望约 600（锁定时长 %v）", secs, lockout)
	}
}

// 锁定时长较短时，Retry-After 也要跟着变短。
func TestLogin_RetryAfterShortLockout(t *testing.T) {
	r := newLoginLimiterEngine(t, middleware.NewLoginLimiter(1, 3*time.Second))

	doJSON(r, http.MethodPost, "/login", map[string]string{"username": "admin", "password": "wrong"})
	w := doJSON(r, http.MethodPost, "/login", map[string]string{"username": "admin", "password": "admin123"})
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("状态码 = %d，期望 429", w.Code)
	}
	secs, err := strconv.Atoi(w.Header().Get("Retry-After"))
	if err != nil {
		t.Fatalf("Retry-After 不是整数秒: %q", w.Header().Get("Retry-After"))
	}
	if secs < 1 || secs > 3 {
		t.Fatalf("Retry-After = %d 秒，期望 1~3", secs)
	}
}

// 剩余时长不足 1 秒时至少返回 1，不能返回 0（会让客户端立刻重试）。
func TestRetryAfterSeconds_RoundsUp(t *testing.T) {
	cases := map[time.Duration]int{
		0:                       1,
		-time.Second:            1,
		400 * time.Millisecond:  1,
		1500 * time.Millisecond: 2,
		10 * time.Minute:        600,
	}
	for in, want := range cases {
		if got := RetryAfterSeconds(in); got != want {
			t.Fatalf("RetryAfterSeconds(%v) = %d，期望 %d", in, got, want)
		}
	}
}

// 成功登录后失败计数应清零，而不是从历史失败次数继续累加。
//
// 阈值 3：先失败 2 次（未达阈值）→ 成功登录（清零）→ 再失败 2 次。
// 若没有清零，累计 4 次会触发锁定；清零后应仍被允许尝试。
func TestLogin_SuccessResetsFailureCount(t *testing.T) {
	r := newLoginLimiterEngine(t, middleware.NewLoginLimiter(3, 0))

	for i := 0; i < 2; i++ {
		doJSON(r, http.MethodPost, "/login", map[string]string{"username": "admin", "password": "wrong"})
	}
	if w := doJSON(r, http.MethodPost, "/login", map[string]string{"username": "admin", "password": "admin123"}); w.Code != http.StatusOK {
		t.Fatalf("成功登录状态码 = %d，期望 200，body=%s", w.Code, w.Body.String())
	}

	// 再失败 2 次：若计数没清零，累计 4 >= 3 会立即锁定
	for i := 0; i < 2; i++ {
		w := doJSON(r, http.MethodPost, "/login", map[string]string{"username": "admin", "password": "wrong"})
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("成功登录后第 %d 次失败状态码 = %d，期望 401（计数应已清零）", i+1, w.Code)
		}
	}
	// 第 5 次仍是密码错，此时累计 3 次达到阈值 → 401，但随后应被锁定
	doJSON(r, http.MethodPost, "/login", map[string]string{"username": "admin", "password": "wrong"})
	w := doJSON(r, http.MethodPost, "/login", map[string]string{"username": "admin", "password": "admin123"})
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("累计失败达到阈值后状态码 = %d，期望 429", w.Code)
	}
}

// 请求体超限应返回 413 而不是笼统的 400。
func TestLogin_BodyTooLarge(t *testing.T) {
	r := newLoginLimiterEngine(t, middleware.NewLoginLimiter(5, 0))

	// 引擎上限 64 字节，这里给一个 1KB 的用户名
	big := make([]byte, 1024)
	for i := range big {
		big[i] = 'a'
	}
	w := doJSON(r, http.MethodPost, "/login", map[string]string{"username": string(big), "password": "x"})
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("状态码 = %d，期望 413，body=%s", w.Code, w.Body.String())
	}
	if !containsSub(w.Body.String(), string(middleware.CodePayloadTooLarge)) {
		t.Fatalf("错误码应为 payload_too_large，body=%s", w.Body.String())
	}
}
