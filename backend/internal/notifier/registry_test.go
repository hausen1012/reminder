package notifier

import (
	"strings"
	"testing"
)

// 回归测试：Registry 的 key 必须覆盖 specs 里声明的所有通道类型。
//
// 漏注册的后果是：用户创建该类型通道后每次发送都失败（错误：未知通道类型）。
func TestRegistry_CoversAllSpecs(t *testing.T) {
	for _, typ := range Types() {
		if _, err := Get(typ); err != nil {
			t.Errorf("通道类型 %q 未注册到 Registry: %v", typ, err)
		}
	}
}

// 反向：Registry 里的每个类型都必须在 specs 中有元数据，否则前端拿不到表单定义。
func TestSpecs_CoversAllRegisteredTypes(t *testing.T) {
	for typ := range Registry {
		if _, ok := SpecOf(typ); !ok {
			t.Errorf("通道类型 %q 已注册但缺少 Spec", typ)
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

// Spec.Type 必须与 map 键一致，且必填字段的键名不能重复。
func TestSpec_SelfConsistent(t *testing.T) {
	for key, s := range specs {
		if s.Type != key {
			t.Errorf("specs[%q].Type = %q", key, s.Type)
		}
		if s.Label == "" {
			t.Errorf("specs[%q] 缺少 Label", key)
		}
		seen := map[string]bool{}
		for _, f := range s.Fields {
			if seen[f.Key] {
				t.Errorf("specs[%q] 字段 %q 重复", key, f.Key)
			}
			seen[f.Key] = true
			if f.Kind == FieldPassword && !strings.HasSuffix(f.Key, encSuffix) {
				t.Errorf("specs[%q] 密码字段 %q 必须以 _enc 结尾", key, f.Key)
			}
			if f.Kind == FieldSelect && len(f.Options) == 0 {
				t.Errorf("specs[%q] 选项字段 %q 缺少 Options", key, f.Key)
			}
		}
	}
}

// specOrder 必须恰好覆盖 specs 里全部类型，且无重复。
func TestSpecs_OrderCoversAll(t *testing.T) {
	if len(specOrder) != len(specs) {
		t.Fatalf("specOrder 有 %d 项，specs 有 %d 项", len(specOrder), len(specs))
	}
	seen := map[string]bool{}
	for _, typ := range specOrder {
		if seen[typ] {
			t.Errorf("specOrder 中 %q 重复", typ)
		}
		seen[typ] = true
		if _, ok := specs[typ]; !ok {
			t.Errorf("specOrder 中的 %q 没有对应 Spec", typ)
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
