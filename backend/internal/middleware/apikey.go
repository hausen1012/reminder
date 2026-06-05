// apikey 提供外部 API Key 鉴权的 Gin 中间件。
//
// 客户端通过 X-API-Key 请求头传递明文 Key，
// 中间件验 sha256、限流后注入 api_key_id 到 gin.Context。
package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// ApiKeyVerifier 是 ApiKeyService.Verify 的函数签名。
type ApiKeyVerifier func(plain string) (apiKeyID uint, ok bool)

// ApiKeyToucher 是 ApiKeyService.TouchLastUsed 的函数签名。
type ApiKeyToucher func(apiKeyID uint)

// RateLimiter 是 per-Key 的内存限流器。
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

// APIKeyAuth 返回中间件。
//
// verify 接收明文返回 keyID；toucher 可选更新 last_used_at；
// limiter 为空时默认 60/min。
func APIKeyAuth(verify ApiKeyVerifier, toucher ApiKeyToucher, limiter *RateLimiter) gin.HandlerFunc {
	if limiter == nil {
		limiter = NewRateLimiter()
	}
	return func(c *gin.Context) {
		key := c.GetHeader("X-API-Key")
		if key == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": gin.H{"code": CodeUnauthorized, "message": "缺少 X-API-Key 请求头"},
			})
			return
		}

		keyID, ok := verify(key)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": gin.H{"code": CodeUnauthorized, "message": "API Key 无效"},
			})
			return
		}

		if !limiter.Allow(keyID, 60) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": gin.H{"code": CodeRateLimited, "message": "请求过于频繁"},
			})
			return
		}

		c.Set("api_key_id", keyID)
		if toucher != nil {
			toucher(keyID)
		}
		c.Next()
	}
}
