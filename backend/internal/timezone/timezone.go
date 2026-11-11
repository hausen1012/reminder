// Package timezone 提供运行时可动态更新的站点时区 Provider。
package timezone

import (
	"fmt"
	"sync"
	"time"
)

// Fallback 是未配置或解析失败时的兜底时区。
const Fallback = "Asia/Shanghai"

// Provider 线程安全地持有站点时区名与解析结果。
type Provider struct {
	mu   sync.RWMutex
	name string
	loc  *time.Location
}

// New 用给定名字构造 Provider；名字为空或无效时回退到 Fallback（镜像缺 tzdata 时退到 UTC）。
func New(name string) *Provider {
	p := &Provider{}
	if name == "" || p.Set(name) != nil {
		if err := p.Set(Fallback); err != nil {
			p.mu.Lock()
			p.name = "UTC"
			p.loc = time.UTC
			p.mu.Unlock()
		}
	}
	return p
}

// NewFixed 用已有的 *time.Location 构造 Provider，主要供测试注入固定时区。
func NewFixed(loc *time.Location) *Provider {
	if loc == nil {
		return New("")
	}
	return &Provider{name: loc.String(), loc: loc}
}

// Get 返回当前时区。返回值不可变，可安全并发使用。
func (p *Provider) Get() *time.Location {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.loc
}

// Name 返回当前时区名。
func (p *Provider) Name() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.name
}

// Set 校验并切换时区；名字为空时回退到 Fallback。
func (p *Provider) Set(name string) error {
	if name == "" {
		name = Fallback
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return fmt.Errorf("未知时区 %q", name)
	}
	p.mu.Lock()
	p.name = name
	p.loc = loc
	p.mu.Unlock()
	return nil
}

// Valid 判断时区名能否被解析，用于入参校验。
func Valid(name string) bool {
	if name == "" {
		return false
	}
	_, err := time.LoadLocation(name)
	return err == nil
}
