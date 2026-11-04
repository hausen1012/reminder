package middleware

import (
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// LoginLimiter 是登录接口的失败计数限流器。
//
// 为什么需要：登录接口不需要任何凭据就能调用，暴露到公网后
// 不设上限就等于允许无限撞库。外部令牌有 per-令牌限流，登录没有。
//
// 计数键是「用户名 + 客户端 IP」而不是单独 IP 或单独用户名：
//   - 只按 IP：同一 NAT 后面的正常用户会被别人的失败计数牵连；
//   - 只按用户名：换 IP 就能绕过，且会被人故意用别人的用户名锁死账号。
//
// 成功登录会清零该键；锁定期间直接拒绝，不进入密码校验，
// 这样 bcrypt 也不会被用来做 CPU 消耗攻击。
//
// 检查与记录由 AuthHandler 显式调用，而不是做成中间件：
// 中间件要拿到用户名就得先读请求体，读掉之后 handler 就解析不到了。
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
	// lastSeen 是该键最近一次被尝试的时间，用于惰性清理。
	// 只靠 failures==0 清理不够：攻击者可以用大量不同用户名各失败一次，
	// 这些槽位永远不归零，map 会无界增长。
	lastSeen time.Time
}

// idleTTL 是槽位的空闲过期时间：超过它没有任何尝试就丢弃。
//
// 取 max(lockout, 1h)：锁定窗口内的计数必须留着，否则锁定会被绕过；
// 而比锁定期更长的闲置槽位已经没有意义，丢掉后用户下次失败从 0 开始数。
func (l *LoginLimiter) idleTTL() time.Duration {
	if l.lockout > time.Hour {
		return l.lockout
	}
	return time.Hour
}

// NewLoginLimiter 构造登录限流器。
//
// maxAttempts <= 0 表示关闭限流。
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

// LoginKey 构造计数键：小写用户名 + 客户端 IP。
//
// 用户名统一小写：数据库查询大小写敏感，但计数不该因为把 admin 写成 Admin
// 就重置；IP 取的是 AuthHandler 收到请求时的 ClientIP（结果取决于
// TRUSTED_PROXIES 配置），不是原始 X-Forwarded-For。
func LoginKey(c *gin.Context, username string) string {
	return strings.ToLower(strings.TrimSpace(username)) + "|" + c.ClientIP()
}

// Allow 判断当前键是否处于锁定状态。未锁定时返回 true。
func (l *LoginLimiter) Allow(key string) bool {
	ok, _ := l.AllowWithRetryAfter(key)
	return ok
}

// AllowWithRetryAfter 同 Allow，并额外返回锁定剩余时长（未锁定时为 0）。
//
// 调用方用它填 Retry-After：写死 60 秒会与实际锁定时长脱节——
// 配置成 15 分钟时，客户端按 60 秒重试只会再吃一次 429。
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
