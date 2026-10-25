package middleware

import (
	"testing"
	"time"
)

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
