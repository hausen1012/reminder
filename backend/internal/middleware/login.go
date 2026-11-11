package middleware

import (
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// LoginLimiter 针对「用户名 + 客户端 IP」提供登录失败限流，锁定期间直接拒绝。
type LoginLimiter struct {
	maxAttempts int
	lockout     time.Duration

	mu    sync.Mutex
	slots map[string]*loginSlot
	calls uint64 // 用于触发惰性清理
}

type loginSlot struct {
	failures  int
	lockedTil time.Time
	// lastSeen 用于惰性清理：攻击者可用大量用户名单次失败让 map 无界增长
	lastSeen time.Time
}

// idleTTL 是槽位的空闲过期时间，取 max(lockout, 1h)：
// 锁定期内的计数必须保留，更久没动静的槽位已无意义。
func (l *LoginLimiter) idleTTL() time.Duration {
	if l.lockout > time.Hour {
		return l.lockout
	}
	return time.Hour
}

// NewLoginLimiter 构造登录限流器；maxAttempts <= 0 表示关闭限流。
func NewLoginLimiter(maxAttempts int, lockout time.Duration) *LoginLimiter {
	if lockout <= 0 {
		lockout = 15 * time.Minute
	}
	return &LoginLimiter{
		maxAttempts: maxAttempts,
		lockout:     lockout,
		slots:       make(map[string]*loginSlot),
	}
}

// LoginKey 构造计数键：小写用户名 + ClientIP。
func LoginKey(c *gin.Context, username string) string {
	return strings.ToLower(strings.TrimSpace(username)) + "|" + c.ClientIP()
}

// Allow 判断当前键是否处于锁定状态。未锁定时返回 true。
func (l *LoginLimiter) Allow(key string) bool {
	ok, _ := l.AllowWithRetryAfter(key)
	return ok
}

// AllowWithRetryAfter 检查是否允许登录，并返回锁定剩余时长。
func (l *LoginLimiter) AllowWithRetryAfter(key string) (bool, time.Duration) {
	if l.maxAttempts <= 0 {
		return true, 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	l.calls++
	// 每 256 次调用做一次惰性清理，否则长期运行时 slots 只增不减。
	if l.calls%256 == 0 {
		ttl := l.idleTTL()
		for k, slot := range l.slots {
			if now.Sub(slot.lastSeen) > ttl {
				delete(l.slots, k)
			}
		}
	}

	slot, ok := l.slots[key]
	if !ok {
		return true, 0
	}
	if now.Sub(slot.lastSeen) > l.idleTTL() {
		// 闲置过久（例如昨天失败过两次）：当作全新的键
		delete(l.slots, key)
		return true, 0
	}
	if slot.lockedTil.IsZero() {
		return true, 0
	}
	if now.After(slot.lockedTil) {
		// 锁定期已过：清零计数，给用户重新尝试的机会
		slot.failures = 0
		slot.lockedTil = time.Time{}
		return true, 0
	}
	return false, slot.lockedTil.Sub(now)
}

// RecordFailure 记一次失败；达到阈值时开始锁定。
func (l *LoginLimiter) RecordFailure(key string) {
	if l.maxAttempts <= 0 {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	slot, ok := l.slots[key]
	if !ok || now.Sub(slot.lastSeen) > l.idleTTL() {
		slot = &loginSlot{}
		l.slots[key] = slot
	}
	slot.lastSeen = now
	slot.failures++
	if slot.failures >= l.maxAttempts {
		slot.lockedTil = now.Add(l.lockout)
		slog.Warn("登录失败次数过多，已临时锁定",
			"key", key, "failures", slot.failures, "lockout", l.lockout)
	}
}

// RecordSuccess 清空该键的失败计数。
func (l *LoginLimiter) RecordSuccess(key string) {
	if l.maxAttempts <= 0 {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.slots, key)
}
