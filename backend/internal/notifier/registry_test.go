package notifier

import "testing"

// 回归测试：Registry 的 key 必须覆盖所有对外暴露的通道类型。
//
// 曾经 "log" 类型在 ChannelService.validType 与前端 ChannelType 中都存在，
// 但没注册到 Registry，导致用户创建「日志输出」通道后每次发送都失败
// （错误：未知通道类型: log）。
func TestRegistry_CoversAllAllowedChannelTypes(t *testing.T) {
	// 与 services.ChannelService.validType 保持一致的类型清单
	allowed := []string{"smtp", "dingtalk", "wecom", "webhook", "log"}

	for _, typ := range allowed {
		if _, err := Get(typ); err != nil {
			t.Errorf("通道类型 %q 未注册到 Registry: %v", typ, err)
		}
	}
}

// 每个注册的 Notifier，其 Type() 返回值必须与注册 key 一致，
// 否则前端按类型回显会错乱。
func TestRegistry_TypeMatchesKey(t *testing.T) {
	for key, n := range Registry {
		if got := n.Type(); got != key {
			t.Errorf("Registry[%q].Type() = %q，两者应一致", key, got)
		}
	}
}

// 未知类型必须返回错误而不是 nil notifier（避免 sendWithRetry 空指针）。
func TestGet_UnknownTypeReturnsError(t *testing.T) {
	n, err := Get("no-such-type")
	if err == nil {
		t.Fatal("未知类型应返回错误")
	}
	if n != nil {
		t.Fatalf("未知类型应返回 nil notifier，实际 %T", n)
	}
}
