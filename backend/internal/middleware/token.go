// token 提供外部令牌鉴权的 Gin 中间件。
//
// 客户端通过 X-AUTH 请求头传递明文令牌，
// 中间件验 sha256、限流后注入 token_id 到 gin.Context。
package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/reminder/backend/internal/logger"
)

// TokenVerifier 是 TokenService.Verify 的函数签名。
type TokenVerifier func(plain string) (tokenID uint, ok bool)

// TokenToucher 是 TokenService.TouchLastUsed 的函数签名。
type TokenToucher func(tokenID uint)

// RateLimiter 是 per-令牌的内存限流器。
type RateLimiter struct {
	mu    sync.Mutex
	slots map[uint]*rateSlot
	calls uint64 // 用于触发惰性清理
}

type rateSlot struct {
	count   int
	resetAt time.Time
}

// NewRateLimiter 构造限流器。
func NewRateLimiter() *RateLimiter {
	return &RateLimiter{slots: make(map[uint]*rateSlot)}
}

// Allow 检查是否允许请求通过。
// limitPerMinute = 每分钟最大请求数。
//
// 顺手清理已过窗口的空闲槽位，避免长期运行时 slots 只增不减。
func (rl *RateLimiter) Allow(keyID uint, limitPerMinute int) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	// 每处理 256 次请求做一次惰性清理，成本可忽略
	rl.calls++
	if rl.calls%256 == 0 {
		for id, slot := range rl.slots {
			if now.After(slot.resetAt) {
				delete(rl.slots, id)
			}
		}
	}

	s, ok := rl.slots[keyID]
	if !ok || now.After(s.resetAt) {
		rl.slots[keyID] = &rateSlot{count: 1, resetAt: now.Add(time.Minute)}
		return true
	}
	if s.count >= limitPerMinute {
		return false
	}
	s.count++
	return true
}

// tokenPrefix 返回令牌的前 8 个字符，用于在日志里区分是哪个令牌。
//
// 只留前缀不足以还原明文（令牌是 24 字节随机串），但足以判断"是不是同一个调用方"。
func tokenPrefix(key string) string {
	if len(key) <= 8 {
		return "***"
	}
	return key[:8] + "…"
}

// TokenAuth 返回中间件。
//
// verify 接收明文返回 tokenID；toucher 可选更新 last_used_at；
// limiter 为空时默认 60/min。
func TokenAuth(verify TokenVerifier, toucher TokenToucher, limiter *RateLimiter) gin.HandlerFunc {
	if limiter == nil {
		limiter = NewRateLimiter()
	}
	return func(c *gin.Context) {
		key := c.GetHeader("X-AUTH")
		if key == "" {
			logger.FromContext(c.Request.Context()).Warn("外部令牌鉴权失败",
				"path", c.Request.URL.Path, "reason", "缺少 X-AUTH 请求头")
			c.AbortWithStatusJSON(http.StatusUnauthorized,
				ErrorBody(CodeUnauthorized, "缺少 X-AUTH 请求头", ""))
			return
		}

		tokenID, ok := verify(key)
		if !ok {
			// 只记令牌前缀，不记明文或哈希：日志本身不该成为凭据来源。
			logger.FromContext(c.Request.Context()).Warn("外部令牌鉴权失败",
				"path", c.Request.URL.Path, "reason", "令牌无效", "token_prefix", tokenPrefix(key))
			c.AbortWithStatusJSON(http.StatusUnauthorized,
				ErrorBody(CodeUnauthorized, "令牌无效", ""))
			return
		}

		if !limiter.Allow(tokenID, 60) {
			logger.FromContext(c.Request.Context()).Warn("外部令牌触发限流",
				"path", c.Request.URL.Path, "token_id", tokenID)
			c.AbortWithStatusJSON(http.StatusTooManyRequests,
				ErrorBody(CodeRateLimited, "请求过于频繁", ""))
			return
		}

		c.Set("token_id", tokenID)
		if toucher != nil {
			toucher(tokenID)
		}
		c.Next()
	}
}
