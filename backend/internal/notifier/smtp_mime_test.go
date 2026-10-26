package notifier

import (
	"strings"
	"testing"
)

// buildMIME：纯文本格式。
func TestBuildMIME_Text(t *testing.T) {
	cfg := SMTPConfig{
		FromAddr: "from@test",
		FromName: "发件人",
		To:       []string{"a@test", "b@test"},
	}
	mime := buildMIME(cfg, Message{Subject: "主题", Body: "正文内容", Format: "text"})

	if !strings.Contains(mime, "Content-Type: text/plain; charset=UTF-8") {
		t.Fatalf("应为 text/plain: %s", mime)
	}
	// FromName 应做 RFC2047 编码
	if !strings.Contains(mime, "=?UTF-8?B?") {
		t.Fatalf("FromName 应做 base64 编码: %s", mime)
	}
	// 主题同样编码
	if !strings.Contains(mime, "Subject: =?UTF-8?B?") {
		t.Fatalf("Subject 应做 base64 编码: %s", mime)
	}
	// 多个收件人逗号分隔
	if !strings.Contains(mime, "To: a@test, b@test") {
		t.Fatalf("To 头不正确: %s", mime)
	}
	if !strings.HasSuffix(mime, "正文内容") {
		t.Fatalf("body 应在末尾: %s", mime)
	}
}

// buildMIME：FromName 为空时只用地址。
func TestBuildMIME_NoFromName(t *testing.T) {
	cfg := SMTPConfig{FromAddr: "from@test", To: []string{"a@test"}}
	mime := buildMIME(cfg, Message{Subject: "S", Body: "B"})

	if !strings.Contains(mime, "From: from@test") {
		t.Fatalf("From 头应为裸地址: %s", mime)
	}
}

// buildMIME：html 格式原样放入 body。
func TestBuildMIME_HTML(t *testing.T) {
	cfg := SMTPConfig{FromAddr: "f@t", To: []string{"a@t"}}
	mime := buildMIME(cfg, Message{Subject: "S", Body: "<p>段落</p>", Format: "html"})

	if !strings.Contains(mime, "Content-Type: text/html; charset=UTF-8") {
		t.Fatalf("应为 text/html: %s", mime)
	}
	if !strings.Contains(mime, "<p>段落</p>") {
		t.Fatalf("html 应原样保留: %s", mime)
	}
}

// buildMIME：markdown 应渲染为 HTML。
func TestBuildMIME_Markdown(t *testing.T) {
	cfg := SMTPConfig{FromAddr: "f@t", To: []string{"a@t"}}
	mime := buildMIME(cfg, Message{Subject: "S", Body: "# 标题", Format: "markdown"})

	if !strings.Contains(mime, "Content-Type: text/html; charset=UTF-8") {
		t.Fatalf("markdown 应转 text/html: %s", mime)
	}
	// goldmark 把 "# 标题" 渲染为 <h1>
	if !strings.Contains(mime, "<h1>") {
		t.Fatalf("markdown 未渲染为 HTML: %s", mime)
	}
}

// renderMarkdownToHTML 应产出 HTML 片段。
func TestRenderMarkdownToHTML(t *testing.T) {
	out := renderMarkdownToHTML("**粗体**")
	if !strings.Contains(out, "<strong>") {
		t.Fatalf("粗体未渲染: %s", out)
	}

	// 空输入产出空串或仅换行
	if got := renderMarkdownToHTML(""); strings.TrimSpace(got) != "" {
		t.Fatalf("空输入应产出空串，实际 %q", got)
	}
}

// resolveSMTPSecurityMode 的补充用例：逐一确认每个分支的返回值。
func TestResolveSMTPSecurityMode_AllBranches(t *testing.T) {
	cases := []struct {
		name string
		cfg  SMTPConfig
		want string
	}{
		{"显式 plain", SMTPConfig{SecurityMode: "plain"}, "plain"},
		{"显式 starttls", SMTPConfig{SecurityMode: "starttls"}, "starttls"},
		{"显式 implicit_tls", SMTPConfig{SecurityMode: "implicit_tls"}, "implicit_tls"},
		{"UseStartTLS", SMTPConfig{UseStartTLS: true}, "starttls"},
		{"端口 465", SMTPConfig{Port: 465}, "implicit_tls"},
		{"默认 plain", SMTPConfig{Port: 587}, "plain"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveSMTPSecurityMode(tc.cfg)
			if err != nil {
				t.Fatalf("不应报错: %v", err)
			}
			if got != tc.want {
				t.Fatalf("得到 %q，期望 %q", got, tc.want)
			}
		})
	}
}
