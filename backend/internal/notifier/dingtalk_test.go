package notifier

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// 钉钉：成功响应（errcode=0）应返回 nil。
func TestDingTalk_SendSuccess(t *testing.T) {
	var captured []byte
	var contentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured, _ = io.ReadAll(r.Body)
		contentType = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
	}))
	defer srv.Close()

	cfg, _ := json.Marshal(DingTalkConfig{WebhookURL: srv.URL})
	err := (&dingtalkNotifier{}).Send(t.Context(), cfg, Message{Subject: "标题", Body: "正文"})
	if err != nil {
		t.Fatalf("发送应成功: %v", err)
	}
	if !strings.Contains(contentType, "application/json") {
		t.Fatalf("Content-Type = %q", contentType)
	}

	var payload map[string]any
	if err := json.Unmarshal(captured, &payload); err != nil {
		t.Fatalf("payload 不是合法 JSON: %v", err)
	}
	if payload["msgtype"] != "text" {
		t.Fatalf("msgtype = %v", payload["msgtype"])
	}
}

// 钉钉：缺少 webhook_url 应返回永久错误。
func TestDingTalk_MissingWebhook(t *testing.T) {
	cfg, _ := json.Marshal(DingTalkConfig{})
	err := (&dingtalkNotifier{}).Send(t.Context(), cfg, Message{Body: "x"})
	if err == nil || !IsPermanent(err) {
		t.Fatalf("缺少 webhook 应为永久错误，得到 %v", err)
	}
}

// 钉钉：配置不是合法 JSON 应返回永久错误。
func TestDingTalk_BadConfig(t *testing.T) {
	err := (&dingtalkNotifier{}).Send(t.Context(), []byte("{bad"), Message{Body: "x"})
	if err == nil || !IsPermanent(err) {
		t.Fatalf("非法配置应为永久错误，得到 %v", err)
	}
}

// 钉钉：业务错误码非 0 应返回永久错误（不重试）。
func TestDingTalk_BusinessError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"errcode":310000,"errmsg":"sign not match"}`))
	}))
	defer srv.Close()

	cfg, _ := json.Marshal(DingTalkConfig{WebhookURL: srv.URL})
	err := (&dingtalkNotifier{}).Send(t.Context(), cfg, Message{Body: "x"})
	if err == nil || !IsPermanent(err) {
		t.Fatalf("业务错误应为永久错误，得到 %v", err)
	}
}

// 钉钉：4xx 永久错误，5xx 可重试。
func TestDingTalk_HTTPStatus(t *testing.T) {
	cases := []struct {
		status    int
		permanent bool
	}{
		{http.StatusBadRequest, true},
		{http.StatusTooManyRequests, true},
		{http.StatusInternalServerError, false},
		{http.StatusBadGateway, false},
	}
	for _, tc := range cases {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
			}))
			defer srv.Close()

			cfg, _ := json.Marshal(DingTalkConfig{WebhookURL: srv.URL})
			err := (&dingtalkNotifier{}).Send(t.Context(), cfg, Message{Body: "x"})
			if err == nil {
				t.Fatal("应返回错误")
			}
			if IsPermanent(err) != tc.permanent {
				t.Fatalf("status=%d 期望 permanent=%v，实际 %v（err=%v）", tc.status, tc.permanent, IsPermanent(err), err)
			}
		})
	}
}

// 钉钉：启用加签时 URL 应追加 timestamp 与 sign 参数。
func TestDingTalk_SignsURL(t *testing.T) {
	var gotQuery url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"errcode":0}`))
	}))
	defer srv.Close()

	cfg, _ := json.Marshal(DingTalkConfig{WebhookURL: srv.URL, Secret: "SECabc123"})
	if err := (&dingtalkNotifier{}).Send(t.Context(), cfg, Message{Body: "x"}); err != nil {
		t.Fatalf("发送失败: %v", err)
	}

	if gotQuery.Get("timestamp") == "" {
		t.Fatal("缺少 timestamp 参数")
	}
	if gotQuery.Get("sign") == "" {
		t.Fatal("缺少 sign 参数")
	}
}

// signDingTalk 对非法 URL 应报错。
func TestSignDingTalk_InvalidURL(t *testing.T) {
	if _, err := signDingTalk("://bad-url", "secret"); err == nil {
		t.Fatal("非法 URL 应报错")
	}
}

// 钉钉 payload：markdown 带 title；at 字段按配置附加。
func TestBuildDingTalkPayload(t *testing.T) {
	// markdown 有标题
	p := buildDingTalkPayload(DingTalkConfig{}, Message{Subject: "我的标题", Body: "**加粗**", Format: "markdown"})
	if p["msgtype"] != "markdown" {
		t.Fatalf("msgtype = %v", p["msgtype"])
	}
	md := p["markdown"].(map[string]any)
	if md["title"] != "我的标题" {
		t.Fatalf("title = %v", md["title"])
	}

	// markdown 无标题时用默认"提醒"
	p = buildDingTalkPayload(DingTalkConfig{}, Message{Body: "x", Format: "markdown"})
	md = p["markdown"].(map[string]any)
	if md["title"] != "提醒" {
		t.Fatalf("默认 title = %v", md["title"])
	}

	// text 格式拼上标题
	p = buildDingTalkPayload(DingTalkConfig{}, Message{Subject: "S", Body: "B", Format: "text"})
	text := p["text"].(map[string]any)
	if text["content"] != "S\nB" {
		t.Fatalf("content = %q", text["content"])
	}

	// html 格式应剥离标签
	p = buildDingTalkPayload(DingTalkConfig{}, Message{Body: "<b>粗</b>", Format: "html"})
	text = p["text"].(map[string]any)
	if strings.Contains(text["content"].(string), "<b>") {
		t.Fatalf("html 未剥离: %v", text["content"])
	}

	// at_all 应附加 at 字段
	p = buildDingTalkPayload(DingTalkConfig{AtAll: true}, Message{Body: "x", Format: "text"})
	if _, ok := p["at"]; !ok {
		t.Fatal("at_all 应附加 at 字段")
	}

	// at_mobiles 应附加 at 字段
	p = buildDingTalkPayload(DingTalkConfig{AtMobiles: []string{"138"}}, Message{Body: "x", Format: "text"})
	if _, ok := p["at"]; !ok {
		t.Fatal("at_mobiles 应附加 at 字段")
	}
}

// base64Encode 应等价于标准 base64。
func TestBase64Encode(t *testing.T) {
	if got := base64Encode("hello"); got != "aGVsbG8=" {
		t.Fatalf("base64Encode = %q", got)
	}
	if got := base64Encode(""); got != "" {
		t.Fatalf("空串应返回空，实际 %q", got)
	}
}

// StripHTML 应去掉所有标签并 trim。
func TestStripHTML(t *testing.T) {
	cases := map[string]string{
		"<p>段落</p>":                  "段落",
		"<a href='x'>链接</a>":         "链接",
		"  <b>粗</b>  ":               "粗",
		"纯文本":                        "纯文本",
		"<div><span>嵌套</span></div>": "嵌套",
	}
	for in, want := range cases {
		if got := StripHTML(in); got != want {
			t.Errorf("StripHTML(%q) = %q, 期望 %q", in, got, want)
		}
	}
}

// StripMarkdown 应去掉常见标记。
func TestStripMarkdown(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"# 标题", "标题"},
		{"**加粗**", "加粗"},
		{"*斜体*", "斜体"},
		{"[链接](http://x)", "链接"},
		{"![图片](http://x)", "图片"},
		{"> 引用", "引用"},
		{"- 列表项", "列表项"},
		{"1. 有序", "有序"},
		{"`代码`", ""},
	}
	for _, tc := range cases {
		if got := StripMarkdown(tc.in); got != tc.want {
			t.Errorf("StripMarkdown(%q) = %q, 期望 %q", tc.in, got, tc.want)
		}
	}
}
