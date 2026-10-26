package notifier

import (
	"testing"
)

// log 通道：各种格式都应正常返回 nil（输出到日志，不调外部服务）。
func TestLogNotifier_Send(t *testing.T) {
	n := &logNotifier{}

	cases := []struct {
		name string
		msg  Message
	}{
		{"纯文本", Message{Subject: "标题", Body: "正文", Format: "text"}},
		{"html", Message{Subject: "标题", Body: "<p>段落</p>", Format: "html"}},
		{"markdown", Message{Subject: "标题", Body: "**粗**", Format: "markdown"}},
		{"空格式", Message{Subject: "", Body: "", Format: ""}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := n.Send(t.Context(), nil, tc.msg); err != nil {
				t.Fatalf("log 通道不应返回错误: %v", err)
			}
		})
	}
}

// log 通道的 Type 必须是 "log"（与 ChannelService.validType、前端 ChannelType 一致）。
func TestLogNotifier_Type(t *testing.T) {
	if got := (&logNotifier{}).Type(); got != "log" {
		t.Fatalf("Type() = %q，期望 log", got)
	}
}
