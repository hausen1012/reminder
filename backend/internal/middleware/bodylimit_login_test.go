package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// newBodyLimitEngine 构造带 BodyLimit 与统一错误处理的最小引擎。
func newBodyLimitEngine(maxBytes int64) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(BodyLimit(maxBytes))
	r.Use(ErrorHandler())
	r.POST("/echo", func(c *gin.Context) {
		var in map[string]any
		if err := c.ShouldBindJSON(&in); err != nil {
			// 真实 handler 走 abortBindErr：超限映射为 413，其余为 400。
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

	// 32 字节：超过 16 字节上限
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

	// httptest.NewRequest 传入一个非 Reader 类型无法产生 chunked，
	// 这里显式把 ContentLength 置为 -1 模拟未声明长度的请求。
	req := httptest.NewRequest(http.MethodPost, "/echo", io.NopCloser(strings.NewReader(
		`{"title":"`+strings.Repeat("a", 64)+`"}`,
	)))
	req.ContentLength = -1
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("状态码 = %d，期望 413（未声明长度的超限请求也要被拦截），body=%s", w.Code, w.Body.String())
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

// maxBytes <= 0 时应完全不限制（保持中间件可安全传入零值）。
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
	// 第 3 次失败后达到阈值
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
	// 计数已清零：再失败一次远未达阈值
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

// 过期槽位应被惰性清理：大量不同用户名各失败一次后，map 不能无界增长。
func TestLoginLimiter_CleansUpIdleSlots(t *testing.T) {
	l := NewLoginLimiter(3, time.Minute)

	// 塞入 300 个「很久以前失败过一次」的槽位
	l.mu.Lock()
	for i := 0; i < 300; i++ {
		l.slots[string(rune('a'+i%26))+itoaTest(i)] = &loginSlot{
			failures: 1,
			lastSeen: time.Now().Add(-2 * time.Hour),
		}
	}
	l.mu.Unlock()

	// 触发一次清理（Allow 每 256 次调用清理一次）
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

// itoaTest 是测试内的小工具，避免为一条断言引入 strconv。
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
