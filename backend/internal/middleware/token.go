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
)

// TokenVerifier 是 TokenService.Verify 的函数签名。
type TokenVerifier func(plain string) (tokenID uint, ok bool)

// TokenToucher 是 TokenService.TouchLastUsed 的函数签名。
type TokenToucher func(tokenID uint)

// RateLimiter 是 per-令牌的内存限流器。
type RateLimiter struct {
	mu    sync.Mutex
	slots map[uint]*rateSlot
}

type rateSlot struct {
	count    int
	resetAt  time.Time
}

// NewRateLimiter 构造限流器。
func NewRateLimiter() *RateLimiter {
	return &RateLimiter{slots: make(map[uint]*rateSlot)}
}

// Allow 检查是否允许请求通过。
// limitPerMinute = 每分钟最大请求数。
func (rl *RateLimiter) Allow(keyID uint, limitPerMinute int) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	s, ok := rl.slots[keyID]
	now := time.Now()
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
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": gin.H{"code": CodeUnauthorized, "message": "缺少 X-AUTH 请求头"},
			})
			return
		}

		tokenID, ok := verify(key)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": gin.H{"code": CodeUnauthorized, "message": "令牌无效"},
			})
			return
		}

		if !limiter.Allow(tokenID, 60) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": gin.H{"code": CodeRateLimited, "message": "请求过于频繁"},
			})
			return
		}

		c.Set("token_id", tokenID)
		if toucher != nil {
			toucher(tokenID)
		}
		c.Next()
	}
}
