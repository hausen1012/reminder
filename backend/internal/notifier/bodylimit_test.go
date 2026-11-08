package notifier

import (
	"strings"
	"testing"
)

func TestLimitBody_NoLimit(t *testing.T) {
	msg := Message{Body: strings.Repeat("字", 10000)}
	got := LimitBody("log", msg)
	if got.Body != msg.Body {
		t.Fatal("无上限的类型不应截断")
	}
}

func TestLimitBody_Truncates(t *testing.T) {
	spec, ok := SpecOf("feishu")
	if !ok || spec.MaxBodyLen <= 0 {
		t.Fatal("feishu 应声明长度上限")
	}
	msg := Message{Body: strings.Repeat("字", spec.MaxBodyLen+50), Vars: map[string]string{"content": "old"}}
	got := LimitBody("feishu", msg)

	if n := len([]rune(got.Body)); n != spec.MaxBodyLen {
		t.Fatalf("截断后长度 = %d，期望 %d", n, spec.MaxBodyLen)
	}
	if !strings.HasSuffix(got.Body, TruncatedSuffix) {
		t.Fatalf("截断后应以省略号结尾: %q", got.Body)
	}
	if got.Vars["content"] != got.Body {
		t.Fatal("Vars[content] 应与截断后的正文一致")
	}
	if msg.Vars["content"] != "old" {
		t.Fatal("不应修改调用方传入的 Vars")
	}
}

func TestLimitBody_ExactlyAtLimit(t *testing.T) {
	spec, _ := SpecOf("feishu")
	body := strings.Repeat("字", spec.MaxBodyLen)
	got := LimitBody("feishu", Message{Body: body})
	if got.Body != body {
		t.Fatal("恰好等于上限时不应截断")
	}
}

func TestLimitBody_UnknownType(t *testing.T) {
	body := strings.Repeat("x", 5000)
	if got := LimitBody("no-such-type", Message{Body: body}); got.Body != body {
		t.Fatal("未知类型不应截断")
	}
}
