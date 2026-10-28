// Package timezone 提供可在运行时修改的站点时区。
//
// 站点时区保存在 config 表（key=timezone）中，启动时覆盖环境变量 TIMEZONE；
// 在设置页修改后通过 Provider.Set 热更新，无需重启进程。
//
// 之所以需要这一层而不是直接传 *time.Location：调度器、投递、确认重发
// 等处都要按"当前站点时区"解释本地时间字符串，跑起来之后时区仍可能变。
package timezone

import (
	"fmt"
	"sync"
	"time"
)

// Fallback 是未配置或解析失败时的兜底时区，与历史默认值保持一致。
const Fallback = "Asia/Shanghai"

// Provider 线程安全地持有站点时区名与解析结果。
type Provider struct {
	mu   sync.RWMutex
	name string
	loc  *time.Location
}

// New 用给定名字构造 Provider；名字为空或无效时回退到 Fallback，
// Fallback 本身也解析不出来（镜像缺 tzdata）时退到 UTC。
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

// UTC 把时间归一化为 UTC。
//
// 落库时间现在由 models.Time 统一转成 Unix 毫秒整数，偏移已经不影响比较与排序；
// 这里保留是为了给需要"明确按 UTC 展示/计算"的调用方一个语义清晰的入口。
func UTC(t time.Time) time.Time {
	return t.UTC()
}

// UTCPtr 归一化可空时间到 UTC，nil 原样返回。
func UTCPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}
