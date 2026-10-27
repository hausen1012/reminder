package timezone

import (
	"testing"
	"time"
)

// 未传名字时回退到默认时区。
func TestNew_EmptyFallsBack(t *testing.T) {
	p := New("")
	if p.Name() != Fallback {
		t.Fatalf("Name() = %q，期望 %q", p.Name(), Fallback)
	}
	if p.Get() == nil {
		t.Fatal("Get() 不应为 nil")
	}
}

// 非法时区名回退到默认时区。
func TestNew_InvalidFallsBack(t *testing.T) {
	p := New("Mars/Olympus")
	if p.Name() != Fallback {
		t.Fatalf("非法时区应回退，实际 %q", p.Name())
	}
}

// 合法时区名应被正确解析。
func TestNew_Valid(t *testing.T) {
	p := New("America/New_York")
	if p.Name() != "America/New_York" {
		t.Fatalf("Name() = %q", p.Name())
	}
	if _, off := time.Now().In(p.Get()).Zone(); off == 0 {
		t.Fatal("纽约偏移不应为 0")
	}
}

// Set 应热更新时区，且 Get 立刻可见。
func TestProvider_SetHotSwap(t *testing.T) {
	p := New("Asia/Shanghai")
	before := p.Get()
	if err := p.Set("Europe/Berlin"); err != nil {
		t.Fatalf("Set 失败: %v", err)
	}
	after := p.Get()
	if before == after {
		t.Fatal("Set 后应返回新的 Location")
	}
	if p.Name() != "Europe/Berlin" {
		t.Fatalf("Name() = %q", p.Name())
	}
}

// Set 空字符串应回退到 Fallback 而不是清空。
func TestProvider_SetEmptyUsesFallback(t *testing.T) {
	p := New("Europe/Berlin")
	if err := p.Set(""); err != nil {
		t.Fatalf("Set 空字符串不应报错: %v", err)
	}
	if p.Name() != Fallback {
		t.Fatalf("Name() = %q，期望 %q", p.Name(), Fallback)
	}
}

// Set 非法时区应报错且不改变当前值。
func TestProvider_SetInvalidKeepsOld(t *testing.T) {
	p := New("Asia/Shanghai")
	if err := p.Set("Not/AZone"); err == nil {
		t.Fatal("非法时区应报错")
	}
	if p.Name() != "Asia/Shanghai" {
		t.Fatalf("非法 Set 不应改变当前时区，实际 %q", p.Name())
	}
}

// NewFixed 直接使用给定的 Location，供测试注入。
func TestNewFixed(t *testing.T) {
	loc := time.FixedZone("CST", 8*3600)
	p := NewFixed(loc)
	if p.Get() != loc {
		t.Fatal("NewFixed 应直接返回传入的 Location")
	}
	// nil 回退到默认时区
	if NewFixed(nil).Name() != Fallback {
		t.Fatal("NewFixed(nil) 应回退到默认时区")
	}
}

// 并发读写不应触发数据竞争（配合 -race 运行）。
func TestProvider_ConcurrentAccess(t *testing.T) {
	p := New("Asia/Shanghai")
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 100; i++ {
			_ = p.Get().String()
			_ = p.Name()
		}
	}()
	for i := 0; i < 100; i++ {
		_ = p.Set("Asia/Tokyo")
		_ = p.Set("Asia/Shanghai")
	}
	<-done
}

// Valid 只对可解析的时区名返回 true。
func TestValid(t *testing.T) {
	if !Valid("Asia/Shanghai") {
		t.Fatal("Asia/Shanghai 应有效")
	}
	if Valid("") {
		t.Fatal("空字符串应无效")
	}
	if Valid("Mars/Olympus") {
		t.Fatal("非法时区应无效")
	}
}
