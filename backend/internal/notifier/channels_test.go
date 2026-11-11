package notifier

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// ==================== webhook_test.go ====================

// webhook：POST 无模板时使用默认 {subject,body}。
func TestWebhook_DefaultPostBody(t *testing.T) {
	var captured map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&captured)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg, _ := json.Marshal(WebhookConfig{Method: "POST", URL: srv.URL})
	if err := (&webhookNotifier{}).Send(t.Context(), cfg, Message{Subject: "S", Body: "B"}); err != nil {
		t.Fatalf("发送失败: %v", err)
	}
	if captured["subject"] != "S" || captured["body"] != "B" {
		t.Fatalf("默认 body 不正确: %v", captured)
	}
}

// webhook：GET 无 query 模板时附带 subject/body 参数。
func TestWebhook_DefaultGetQuery(t *testing.T) {
	var q map[string][]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q = r.URL.Query()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg, _ := json.Marshal(WebhookConfig{Method: "GET", URL: srv.URL})
	if err := (&webhookNotifier{}).Send(t.Context(), cfg, Message{Subject: "S", Body: "B"}); err != nil {
		t.Fatalf("发送失败: %v", err)
	}
	if len(q["subject"]) == 0 || q["subject"][0] != "S" {
		t.Fatalf("缺少 subject 参数: %v", q)
	}
	if len(q["body"]) == 0 || q["body"][0] != "B" {
		t.Fatalf("缺少 body 参数: %v", q)
	}
}

// webhook：自定义 headers 与 ContentType 应生效。
func TestWebhook_CustomHeaders(t *testing.T) {
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg, _ := json.Marshal(WebhookConfig{
		Method:      "POST",
		URL:         srv.URL,
		Headers:     map[string]string{"X-Custom": "v1"},
		ContentType: "text/plain",
	})
	if err := (&webhookNotifier{}).Send(t.Context(), cfg, Message{Body: "x"}); err != nil {
		t.Fatalf("发送失败: %v", err)
	}
	if got.Get("X-Custom") != "v1" {
		t.Fatalf("自定义 header 未生效: %v", got)
	}
	if got.Get("Content-Type") != "text/plain" {
		t.Fatalf("Content-Type = %q", got.Get("Content-Type"))
	}
}

// webhook：非法 method / 非法 URL → 永久错误。
func TestWebhook_InvalidConfig(t *testing.T) {
	cases := []WebhookConfig{
		{Method: "PUT", URL: "http://x"},
		{Method: "POST", URL: ""},
		{Method: "POST", URL: "://bad"},
	}
	for i, c := range cases {
		cfg, _ := json.Marshal(c)
		err := (&webhookNotifier{}).Send(t.Context(), cfg, Message{Body: "x"})
		if err == nil || !IsPermanent(err) {
			t.Fatalf("第 %d 种非法配置应为永久错误，得到 %v", i, err)
		}
	}

	if err := (&webhookNotifier{}).Send(t.Context(), []byte(`{bad`), Message{Body: "x"}); err == nil || !IsPermanent(err) {
		t.Fatalf("非法 JSON 应为永久错误，得到 %v", err)
	}
}

// webhook：非 JSON 模板的回退渲染。
func TestWebhook_NonJSONTemplate(t *testing.T) {
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg, _ := json.Marshal(WebhookConfig{
		Method:       "POST",
		URL:          srv.URL,
		BodyTemplate: "text={{title}}",
	})
	err := (&webhookNotifier{}).Send(t.Context(), cfg, Message{Vars: map[string]string{"title": "标题"}})
	if err != nil {
		t.Fatalf("发送失败: %v", err)
	}
	if !strings.Contains(body, "标题") {
		t.Fatalf("模板未渲染: %s", body)
	}
}

// buildPostBody 覆盖三条分支：空模板、合法 JSON、非 JSON。
func TestBuildPostBody(t *testing.T) {
	// 空模板
	out, err := buildPostBody("", Message{Subject: "S", Body: "B"})
	if err != nil {
		t.Fatalf("空模板不应报错: %v", err)
	}
	var m map[string]string
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("输出不是合法 JSON: %v", err)
	}
	if m["subject"] != "S" || m["body"] != "B" {
		t.Fatalf("默认结构不正确: %v", m)
	}

	// 合法 JSON 模板
	out, err = buildPostBody(`{"a":"{{x}}"}`, Message{Vars: map[string]string{"x": "1"}})
	if err != nil {
		t.Fatalf("JSON 模板不应报错: %v", err)
	}
	if !strings.Contains(string(out), `"a":"1"`) {
		t.Fatalf("JSON 模板未渲染: %s", out)
	}

	// 非 JSON 模板
	out, err = buildPostBody("plain {{x}}", Message{Vars: map[string]string{"x": "1"}})
	if err != nil {
		t.Fatalf("非 JSON 模板不应报错: %v", err)
	}
	if string(out) != "plain 1" {
		t.Fatalf("非 JSON 渲染结果 = %q", out)
	}
}

// walkAndRender 应处理数组与嵌套对象。
func TestWalkAndRender(t *testing.T) {
	var node any
	_ = json.Unmarshal([]byte(`{"a":"{{x}}","b":["{{x}}",{"c":"{{x}}"}],"d":1}`), &node)
	walkAndRender(&node, map[string]string{"x": "v"})

	out, _ := json.Marshal(node)
	s := string(out)
	if strings.Contains(s, "{{x}}") {
		t.Fatalf("仍有未渲染占位符: %s", s)
	}
	if !strings.Contains(s, `"a":"v"`) {
		t.Fatalf("顶层字段未渲染: %s", s)
	}
	if !strings.Contains(s, `"c":"v"`) {
		t.Fatalf("嵌套字段未渲染: %s", s)
	}
}

// Render 对未知占位符应原样保留；RenderMap 处理 nil 输入。
func TestTemplateRenderEdge(t *testing.T) {
	if got := Render("", map[string]string{"x": "1"}); got != "" {
		t.Fatalf("空模板应返回空，实际 %q", got)
	}
	if got := Render("{{missing}}", map[string]string{}); got != "{{missing}}" {
		t.Fatalf("未知占位符应原样保留，实际 %q", got)
	}
	if got := RenderMap(nil, map[string]string{}); got != nil {
		t.Fatalf("nil 输入应返回 nil，实际 %v", got)
	}
}

// ==================== dingtalk_test.go ====================

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

// 钉钉：4xx 永久错误，429 限流与 5xx 可重试。
func TestDingTalk_HTTPStatus(t *testing.T) {
	cases := []struct {
		status    int
		permanent bool
	}{
		{http.StatusBadRequest, true},
		{http.StatusTooManyRequests, false},
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

// AppendLink 按内容格式把链接追加到正文。
func TestAppendLink(t *testing.T) {
	cases := []struct {
		format string
		want   string
	}{
		{"text", "正文\n点击确认提醒：https://x/c/t"},
		{"markdown", "正文\n\n[点击确认提醒](https://x/c/t)"},
		{"html", `正文<br><a href="https://x/c/t">点击确认提醒</a>`},
	}
	for _, tc := range cases {
		if got := AppendLink("正文", tc.format, "https://x/c/t", "点击确认提醒"); got != tc.want {
			t.Fatalf("AppendLink(%q) = %q，期望 %q", tc.format, got, tc.want)
		}
	}
}

// ==================== wecom_test.go ====================

// 企微：发送成功。
func TestWeCom_SendSuccess(t *testing.T) {
	var captured []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
	}))
	defer srv.Close()

	cfg, _ := json.Marshal(WeComConfig{WebhookURL: srv.URL})
	if err := (&wecomNotifier{}).Send(t.Context(), cfg, Message{Subject: "S", Body: "B"}); err != nil {
		t.Fatalf("发送应成功: %v", err)
	}
	if !strings.Contains(string(captured), `"msgtype":"text"`) {
		t.Fatalf("payload 不正确: %s", captured)
	}
}

// 企微：缺少 webhook_url / 非法配置 → 永久错误。
func TestWeCom_ConfigErrors(t *testing.T) {
	if err := (&wecomNotifier{}).Send(t.Context(), []byte(`{}`), Message{Body: "x"}); err == nil || !IsPermanent(err) {
		t.Fatalf("缺少 webhook 应为永久错误，得到 %v", err)
	}
	if err := (&wecomNotifier{}).Send(t.Context(), []byte(`{bad`), Message{Body: "x"}); err == nil || !IsPermanent(err) {
		t.Fatalf("非法配置应为永久错误，得到 %v", err)
	}
}

// 企微：业务错误码非 0 → 永久错误。
func TestWeCom_BusinessError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"errcode":93000,"errmsg":"invalid webhook url"}`))
	}))
	defer srv.Close()

	cfg, _ := json.Marshal(WeComConfig{WebhookURL: srv.URL})
	err := (&wecomNotifier{}).Send(t.Context(), cfg, Message{Body: "x"})
	if err == nil || !IsPermanent(err) {
		t.Fatalf("业务错误应为永久错误，得到 %v", err)
	}
}

// 企微：4xx 永久 / 5xx 可重试。
func TestWeCom_HTTPStatus(t *testing.T) {
	cases := []struct {
		status    int
		permanent bool
	}{
		{http.StatusBadRequest, true},
		{http.StatusInternalServerError, false},
		{http.StatusServiceUnavailable, false},
	}
	for _, tc := range cases {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
			}))
			defer srv.Close()

			cfg, _ := json.Marshal(WeComConfig{WebhookURL: srv.URL})
			err := (&wecomNotifier{}).Send(t.Context(), cfg, Message{Body: "x"})
			if err == nil {
				t.Fatal("应返回错误")
			}
			if IsPermanent(err) != tc.permanent {
				t.Fatalf("status=%d 期望 permanent=%v", tc.status, tc.permanent)
			}
		})
	}
}

// 企微 payload：markdown / text / html / @人。
func TestBuildWeComPayload(t *testing.T) {
	// markdown
	p := buildWeComPayload(WeComConfig{}, Message{Body: "**粗**", Format: "markdown"})
	if p["msgtype"] != "markdown" {
		t.Fatalf("msgtype = %v", p["msgtype"])
	}

	// text 带标题
	p = buildWeComPayload(WeComConfig{}, Message{Subject: "S", Body: "B", Format: "text"})
	text := p["text"].(map[string]any)
	if text["content"] != "S\nB" {
		t.Fatalf("content = %q", text["content"])
	}

	// html 剥标签
	p = buildWeComPayload(WeComConfig{}, Message{Body: "<b>x</b>", Format: "html"})
	text = p["text"].(map[string]any)
	if strings.Contains(text["content"].(string), "<b>") {
		t.Fatalf("html 未剥离: %v", text["content"])
	}

	// @ 指定成员
	p = buildWeComPayload(WeComConfig{MentionedList: []string{"u1"}}, Message{Body: "x"})
	text = p["text"].(map[string]any)
	if _, ok := text["mentioned_list"]; !ok {
		t.Fatal("应带 mentioned_list")
	}

	// @ 手机号
	p = buildWeComPayload(WeComConfig{MentionedMobileList: []string{"138"}}, Message{Body: "x"})
	text = p["text"].(map[string]any)
	if _, ok := text["mentioned_mobile_list"]; !ok {
		t.Fatal("应带 mentioned_mobile_list")
	}
}

// ==================== feishu_test.go ====================

// 飞书：成功响应（code=0）应返回 nil，并带上文本消息结构。
func TestFeishu_SendSuccess(t *testing.T) {
	var captured []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"code":0,"msg":"success"}`))
	}))
	defer srv.Close()

	cfg, _ := json.Marshal(FeishuConfig{WebhookURL: srv.URL})
	err := (&feishuNotifier{}).Send(t.Context(), cfg, Message{Subject: "标题", Body: "正文"})
	if err != nil {
		t.Fatalf("发送应成功: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(captured, &payload); err != nil {
		t.Fatalf("payload 不是合法 JSON: %v", err)
	}
	if payload["msg_type"] != "text" {
		t.Fatalf("msg_type = %v", payload["msg_type"])
	}
	content, _ := payload["content"].(map[string]any)
	text, _ := content["text"].(string)
	if !strings.Contains(text, "标题") || !strings.Contains(text, "正文") {
		t.Fatalf("text = %q，应包含标题与正文", text)
	}
}

// 飞书：html 正文应剥掉标签。
func TestFeishu_StripsHTML(t *testing.T) {
	var captured []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte(`{"code":0}`))
	}))
	defer srv.Close()

	cfg, _ := json.Marshal(FeishuConfig{WebhookURL: srv.URL})
	err := (&feishuNotifier{}).Send(t.Context(), cfg, Message{Body: "<b>粗体</b>", Format: "html"})
	if err != nil {
		t.Fatalf("发送应成功: %v", err)
	}
	if strings.Contains(string(captured), "<b>") {
		t.Fatalf("正文不应包含 HTML 标签: %s", captured)
	}
}

// 飞书：开启 @所有人 时应在正文前加 at 标签。
func TestFeishu_AtAll(t *testing.T) {
	var captured []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte(`{"code":0}`))
	}))
	defer srv.Close()

	cfg, _ := json.Marshal(FeishuConfig{WebhookURL: srv.URL, AtAll: true})
	if err := (&feishuNotifier{}).Send(t.Context(), cfg, Message{Body: "正文"}); err != nil {
		t.Fatalf("发送应成功: %v", err)
	}
	var payload struct {
		Content struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(captured, &payload); err != nil {
		t.Fatalf("payload 不是合法 JSON: %v", err)
	}
	if !strings.HasPrefix(payload.Content.Text, `<at user_id="all">`) {
		t.Fatalf("text = %q，应以 @所有人 标签开头", payload.Content.Text)
	}
}

// 飞书：配置了密钥时请求体应带 timestamp 与 sign。
func TestFeishu_Signature(t *testing.T) {
	var captured []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte(`{"code":0}`))
	}))
	defer srv.Close()

	cfg, _ := json.Marshal(FeishuConfig{WebhookURL: srv.URL, Secret: "s3cret"})
	if err := (&feishuNotifier{}).Send(t.Context(), cfg, Message{Body: "x"}); err != nil {
		t.Fatalf("发送应成功: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(captured, &payload); err != nil {
		t.Fatalf("payload 不是合法 JSON: %v", err)
	}
	if payload["timestamp"] == nil || payload["sign"] == "" {
		t.Fatalf("应带 timestamp 与 sign: %s", captured)
	}
}

// 飞书：业务错误码非 0 应返回永久错误。
func TestFeishu_BusinessError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":19021,"msg":"sign match fail"}`))
	}))
	defer srv.Close()

	cfg, _ := json.Marshal(FeishuConfig{WebhookURL: srv.URL})
	err := (&feishuNotifier{}).Send(t.Context(), cfg, Message{Body: "x"})
	if err == nil || !IsPermanent(err) {
		t.Fatalf("业务错误应为永久错误，得到 %v", err)
	}
}

// 飞书：5xx 可重试，4xx 永久错误，429 限流可重试。
func TestFeishu_HTTPStatus(t *testing.T) {
	cases := []struct {
		status    int
		permanent bool
	}{
		{http.StatusTooManyRequests, false},
		{http.StatusInternalServerError, false},
		{http.StatusBadRequest, true},
	}
	for _, tc := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tc.status)
		}))
		cfg, _ := json.Marshal(FeishuConfig{WebhookURL: srv.URL})
		err := (&feishuNotifier{}).Send(t.Context(), cfg, Message{Body: "x"})
		if err == nil {
			srv.Close()
			t.Fatalf("%d 状态码应返回错误", tc.status)
		}
		if got := IsPermanent(err); got != tc.permanent {
			srv.Close()
			t.Fatalf("%d 状态码 IsPermanent = %v，期望 %v", tc.status, got, tc.permanent)
		}
		srv.Close()
	}
}

// 飞书：缺少 webhook 或配置非法应返回永久错误。
func TestFeishu_MissingWebhook(t *testing.T) {
	cfg, _ := json.Marshal(FeishuConfig{})
	err := (&feishuNotifier{}).Send(t.Context(), cfg, Message{Body: "x"})
	if err == nil || !IsPermanent(err) {
		t.Fatalf("缺少 webhook 应为永久错误，得到 %v", err)
	}

	err = (&feishuNotifier{}).Send(t.Context(), []byte("{bad"), Message{Body: "x"})
	if err == nil || !IsPermanent(err) {
		t.Fatalf("非法配置应为永久错误，得到 %v", err)
	}
}

// 飞书：markdown 正文应去掉标记后按纯文本发送。
func TestFeishu_StripsMarkdown(t *testing.T) {
	var captured []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte(`{"code":0}`))
	}))
	defer srv.Close()

	cfg, _ := json.Marshal(FeishuConfig{WebhookURL: srv.URL})
	err := (&feishuNotifier{}).Send(t.Context(), cfg, Message{Body: "**粗体**", Format: "markdown"})
	if err != nil {
		t.Fatalf("发送应成功: %v", err)
	}
	if strings.Contains(string(captured), "**") {
		t.Fatalf("正文不应包含 markdown 标记: %s", captured)
	}
}

// ==================== bark_test.go ====================

func TestBark_SendSuccess(t *testing.T) {
	var captured []byte
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured, _ = io.ReadAll(r.Body)
		path = r.URL.Path
		_, _ = w.Write([]byte(`{"code":200,"message":"success"}`))
	}))
	defer srv.Close()

	cfg, _ := json.Marshal(BarkConfig{
		DeviceKey: "key123",
		ServerURL: srv.URL,
		Group:     "提醒",
		Sound:     "alarm",
		Icon:      "https://example.com/i.png",
	})
	err := (&barkNotifier{}).Send(t.Context(), cfg, Message{Subject: "标题", Body: "正文"})
	if err != nil {
		t.Fatalf("发送应成功: %v", err)
	}
	if path != "/push" {
		t.Fatalf("path = %q，期望 /push", path)
	}

	var payload map[string]any
	if err := json.Unmarshal(captured, &payload); err != nil {
		t.Fatalf("payload 不是合法 JSON: %v", err)
	}
	if payload["device_key"] != "key123" || payload["title"] != "标题" || payload["body"] != "正文" {
		t.Fatalf("payload = %s", captured)
	}
	if payload["group"] != "提醒" || payload["sound"] != "alarm" || payload["icon"] != "https://example.com/i.png" {
		t.Fatalf("可选字段未带上: %s", captured)
	}
}

// 未配置服务端时使用官方地址。
func TestBark_DefaultServer(t *testing.T) {
	if DefaultBarkServer != "https://api.day.app" {
		t.Fatalf("默认服务端 = %q", DefaultBarkServer)
	}
}

// 服务端地址结尾斜杠应被去掉，避免拼出 //push。
func TestBark_TrimsTrailingSlash(t *testing.T) {
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_, _ = w.Write([]byte(`{"code":200}`))
	}))
	defer srv.Close()

	cfg, _ := json.Marshal(BarkConfig{DeviceKey: "k", ServerURL: srv.URL + "/"})
	if err := (&barkNotifier{}).Send(t.Context(), cfg, Message{Body: "x"}); err != nil {
		t.Fatalf("发送应成功: %v", err)
	}
	if path != "/push" {
		t.Fatalf("path = %q，期望 /push", path)
	}
}

func TestBark_BusinessError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":400,"message":"invalid device key"}`))
	}))
	defer srv.Close()

	cfg, _ := json.Marshal(BarkConfig{DeviceKey: "k", ServerURL: srv.URL})
	err := (&barkNotifier{}).Send(t.Context(), cfg, Message{Body: "x"})
	if err == nil || !IsPermanent(err) {
		t.Fatalf("业务错误应为永久错误，得到 %v", err)
	}
}

func TestBark_MissingDeviceKey(t *testing.T) {
	cfg, _ := json.Marshal(BarkConfig{ServerURL: "https://example.com"})
	err := (&barkNotifier{}).Send(t.Context(), cfg, Message{Body: "x"})
	if err == nil || !IsPermanent(err) {
		t.Fatalf("缺少 device key 应为永久错误，得到 %v", err)
	}
}

// html / markdown 正文应降级为纯文本。
func TestBark_StripsFormatting(t *testing.T) {
	cases := []struct {
		name   string
		body   string
		format string
		want   string
		bad    string
	}{
		{"html", "<b>粗体</b>", "html", "粗体", "<b>"},
		{"markdown", "**粗体**", "markdown", "粗体", "**"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var captured []byte
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				captured, _ = io.ReadAll(r.Body)
				_, _ = w.Write([]byte(`{"code":200}`))
			}))
			defer srv.Close()

			cfg, _ := json.Marshal(BarkConfig{DeviceKey: "k", ServerURL: srv.URL})
			if err := (&barkNotifier{}).Send(t.Context(), cfg, Message{Body: tc.body, Format: tc.format}); err != nil {
				t.Fatalf("发送应成功: %v", err)
			}
			var payload struct {
				Body string `json:"body"`
			}
			_ = json.Unmarshal(captured, &payload)
			if payload.Body != tc.want {
				t.Fatalf("body = %q，期望 %q", payload.Body, tc.want)
			}
			if strings.Contains(string(captured), tc.bad) {
				t.Fatalf("不应包含格式标记: %s", captured)
			}
		})
	}
}

// ==================== ntfy_test.go ====================

func TestNtfy_SendSuccess(t *testing.T) {
	var (
		captured      string
		path          string
		title         string
		priority      string
		tags          string
		authorization string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		captured = string(b)
		path = r.URL.Path
		title = r.Header.Get("Title")
		priority = r.Header.Get("Priority")
		tags = r.Header.Get("Tags")
		authorization = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg, _ := json.Marshal(NtfyConfig{
		ServerURL: srv.URL,
		Topic:     "alerts",
		Token:     "tk_123",
		Priority:  "high",
		Tags:      "warning, robot",
	})
	err := (&ntfyNotifier{}).Send(t.Context(), cfg, Message{Subject: "标题", Body: "正文"})
	if err != nil {
		t.Fatalf("发送应成功: %v", err)
	}
	if path != "/alerts" {
		t.Fatalf("path = %q，期望 /alerts", path)
	}
	if captured != "正文" {
		t.Fatalf("正文 = %q", captured)
	}
	if title != "标题" || priority != "high" || tags != "warning,robot" {
		t.Fatalf("header 不符合预期: title=%q priority=%q tags=%q", title, priority, tags)
	}
	if authorization != "Bearer tk_123" {
		t.Fatalf("Authorization = %q", authorization)
	}
}

// priority=default 时不发该头，交给服务端默认值。
func TestNtfy_DefaultPriorityOmitted(t *testing.T) {
	var priority string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		priority = r.Header.Get("Priority")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg, _ := json.Marshal(NtfyConfig{ServerURL: srv.URL, Topic: "t", Priority: "default"})
	if err := (&ntfyNotifier{}).Send(t.Context(), cfg, Message{Body: "x"}); err != nil {
		t.Fatalf("发送应成功: %v", err)
	}
	if priority != "" {
		t.Fatalf("不应发送 Priority 头，得到 %q", priority)
	}
}

// 无令牌时不发 Authorization 头。
func TestNtfy_NoToken(t *testing.T) {
	var hasAuth bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, hasAuth = r.Header["Authorization"]
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg, _ := json.Marshal(NtfyConfig{ServerURL: srv.URL, Topic: "t"})
	if err := (&ntfyNotifier{}).Send(t.Context(), cfg, Message{Body: "x"}); err != nil {
		t.Fatalf("发送应成功: %v", err)
	}
	if hasAuth {
		t.Fatal("无令牌时不应带 Authorization 头")
	}
}

func TestNtfy_MissingTopic(t *testing.T) {
	cfg, _ := json.Marshal(NtfyConfig{ServerURL: "https://example.com"})
	err := (&ntfyNotifier{}).Send(t.Context(), cfg, Message{Body: "x"})
	if err == nil || !IsPermanent(err) {
		t.Fatalf("缺少 topic 应为永久错误，得到 %v", err)
	}
}

// 4xx 永久错误，429 与 5xx 可重试。
func TestNtfy_ErrorClassification(t *testing.T) {
	cases := []struct {
		status    int
		permanent bool
	}{
		{http.StatusTooManyRequests, false},
		{http.StatusBadGateway, false},
		{http.StatusUnauthorized, true},
		{http.StatusNotFound, true},
	}
	for _, tc := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tc.status)
		}))
		cfg, _ := json.Marshal(NtfyConfig{ServerURL: srv.URL, Topic: "t"})
		err := (&ntfyNotifier{}).Send(t.Context(), cfg, Message{Body: "x"})
		if err == nil {
			srv.Close()
			t.Fatalf("%d 状态码应返回错误", tc.status)
		}
		if got := IsPermanent(err); got != tc.permanent {
			srv.Close()
			t.Fatalf("%d 状态码 IsPermanent = %v，期望 %v", tc.status, got, tc.permanent)
		}
		srv.Close()
	}
}

// 服务端地址结尾斜杠应被去掉。
func TestNtfy_TrimsTrailingSlash(t *testing.T) {
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg, _ := json.Marshal(NtfyConfig{ServerURL: srv.URL + "/", Topic: "t"})
	if err := (&ntfyNotifier{}).Send(t.Context(), cfg, Message{Body: "x"}); err != nil {
		t.Fatalf("发送应成功: %v", err)
	}
	if path != "/t" {
		t.Fatalf("path = %q，期望 /t", path)
	}
}

// html / markdown 正文应降级为纯文本。
func TestNtfy_StripsFormatting(t *testing.T) {
	cases := []struct {
		body   string
		format string
		want   string
	}{
		{"<b>粗体</b>", "html", "粗体"},
		{"**粗体**", "markdown", "粗体"},
	}
	for _, tc := range cases {
		var captured string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			captured = string(b)
			w.WriteHeader(http.StatusOK)
		}))
		cfg, _ := json.Marshal(NtfyConfig{ServerURL: srv.URL, Topic: "t"})
		if err := (&ntfyNotifier{}).Send(t.Context(), cfg, Message{Body: tc.body, Format: tc.format}); err != nil {
			srv.Close()
			t.Fatalf("发送应成功: %v", err)
		}
		if captured != tc.want {
			srv.Close()
			t.Fatalf("正文 = %q，期望 %q", captured, tc.want)
		}
		srv.Close()
	}
}

func TestNormalizeCSV(t *testing.T) {
	if got := normalizeCSV(" a , ,b "); got != "a,b" {
		t.Fatalf("normalizeCSV = %q", got)
	}
	if got := normalizeCSV("   "); got != "" {
		t.Fatalf("空输入应得到空串，得到 %q", got)
	}
}

// ==================== apprise_test.go ====================

// 状态化模式：只配 key 时请求 /notify/{key}，请求体不带 urls。
func TestApprise_KeyMode(t *testing.T) {
	var (
		captured []byte
		path     string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured, _ = io.ReadAll(r.Body)
		path = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg, _ := json.Marshal(AppriseConfig{APIURL: srv.URL, Key: "abc123"})
	err := (&appriseNotifier{}).Send(t.Context(), cfg, Message{Subject: "标题", Body: "正文"})
	if err != nil {
		t.Fatalf("发送应成功: %v", err)
	}
	if path != "/notify/abc123" {
		t.Fatalf("path = %q，期望 /notify/abc123", path)
	}
	var payload map[string]any
	if err := json.Unmarshal(captured, &payload); err != nil {
		t.Fatalf("payload 不是合法 JSON: %v", err)
	}
	if _, ok := payload["urls"]; ok {
		t.Fatalf("key 模式不应带 urls: %s", captured)
	}
	if payload["title"] != "标题" || payload["body"] != "正文" || payload["type"] != "info" {
		t.Fatalf("payload = %s", captured)
	}
}

// 无状态模式：配了 urls 时请求 /notify（key 可留空），请求体带 urls。
func TestApprise_StatelessMode(t *testing.T) {
	var (
		captured []byte
		path     string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured, _ = io.ReadAll(r.Body)
		path = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg, _ := json.Marshal(AppriseConfig{
		APIURL: srv.URL + "/",
		URLs:   "mailto://a:b@example.com\n\n  discord://id/token  \n",
		Type:   "warning",
	})
	if err := (&appriseNotifier{}).Send(t.Context(), cfg, Message{Body: "正文"}); err != nil {
		t.Fatalf("发送应成功: %v", err)
	}
	if path != "/notify" {
		t.Fatalf("path = %q，期望 /notify", path)
	}
	var payload map[string]any
	if err := json.Unmarshal(captured, &payload); err != nil {
		t.Fatalf("payload 不是合法 JSON: %v", err)
	}
	if payload["urls"] != "mailto://a:b@example.com,discord://id/token" {
		t.Fatalf("urls = %v", payload["urls"])
	}
	if payload["type"] != "warning" {
		t.Fatalf("type = %v", payload["type"])
	}
}

// key 与 urls 都为空的配置应被拒绝。
func TestApprise_RequiresKeyOrURLs(t *testing.T) {
	cfg, _ := json.Marshal(AppriseConfig{APIURL: "http://example.com"})
	err := (&appriseNotifier{}).Send(t.Context(), cfg, Message{Body: "x"})
	if err == nil || !IsPermanent(err) {
		t.Fatalf("缺少 key/urls 应为永久错误，得到 %v", err)
	}
}

func TestApprise_MissingAPIURL(t *testing.T) {
	cfg, _ := json.Marshal(AppriseConfig{Key: "k"})
	err := (&appriseNotifier{}).Send(t.Context(), cfg, Message{Body: "x"})
	if err == nil || !IsPermanent(err) {
		t.Fatalf("缺少 api_url 应为永久错误，得到 %v", err)
	}
}

// 424 表示没有任何目标发送成功，属于永久错误；503 可重试。
func TestApprise_StatusClassification(t *testing.T) {
	cases := []struct {
		status    int
		permanent bool
	}{
		{http.StatusFailedDependency, true},
		{http.StatusServiceUnavailable, false},
		{http.StatusInternalServerError, false},
		{http.StatusBadRequest, true},
		{http.StatusUnauthorized, true},
	}
	for _, tc := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tc.status)
		}))
		cfg, _ := json.Marshal(AppriseConfig{APIURL: srv.URL, Key: "k"})
		err := (&appriseNotifier{}).Send(t.Context(), cfg, Message{Body: "x"})
		if err == nil {
			srv.Close()
			t.Fatalf("%d 状态码应返回错误", tc.status)
		}
		if got := IsPermanent(err); got != tc.permanent {
			srv.Close()
			t.Fatalf("%d 状态码 IsPermanent = %v，期望 %v", tc.status, got, tc.permanent)
		}
		srv.Close()
	}
}

// 未知消息类型降级为 info。
func TestApprise_NormalizeType(t *testing.T) {
	if got := normalizeAppriseType("unknown"); got != "info" {
		t.Fatalf("normalizeAppriseType = %q", got)
	}
	if got := normalizeAppriseType("failure"); got != "failure" {
		t.Fatalf("normalizeAppriseType = %q", got)
	}
}

// html / markdown 正文应降级为纯文本。
func TestApprise_StripsFormatting(t *testing.T) {
	cases := []struct {
		body   string
		format string
		want   string
	}{
		{"<b>粗体</b>", "html", "粗体"},
		{"**粗体**", "markdown", "粗体"},
	}
	for _, tc := range cases {
		var captured []byte
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			captured, _ = io.ReadAll(r.Body)
			w.WriteHeader(http.StatusOK)
		}))
		cfg, _ := json.Marshal(AppriseConfig{APIURL: srv.URL, Key: "k"})
		if err := (&appriseNotifier{}).Send(t.Context(), cfg, Message{Body: tc.body, Format: tc.format}); err != nil {
			srv.Close()
			t.Fatalf("发送应成功: %v", err)
		}
		var payload struct {
			Body string `json:"body"`
		}
		_ = json.Unmarshal(captured, &payload)
		if payload.Body != tc.want {
			srv.Close()
			t.Fatalf("body = %q，期望 %q", payload.Body, tc.want)
		}
		srv.Close()
	}
}

// ==================== telegram_test.go ====================

// withTelegramBase 把 Bot API 根地址指向本地测试服务。
func withTelegramBase(t *testing.T, base string) {
	t.Helper()
	old := telegramAPIBase
	telegramAPIBase = base
	t.Cleanup(func() { telegramAPIBase = old })
}

func TestTelegram_SendSuccess(t *testing.T) {
	var captured []byte
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured, _ = io.ReadAll(r.Body)
		path = r.URL.Path
		_, _ = w.Write([]byte(`{"ok":true,"result":{}}`))
	}))
	defer srv.Close()
	withTelegramBase(t, srv.URL)

	cfg, _ := json.Marshal(TelegramConfig{BotToken: "123:abc", ChatID: "42"})
	err := (&telegramNotifier{}).Send(t.Context(), cfg, Message{Subject: "标题", Body: "正文"})
	if err != nil {
		t.Fatalf("发送应成功: %v", err)
	}
	if !strings.HasSuffix(path, "/sendMessage") || !strings.Contains(path, "123:abc") {
		t.Fatalf("path = %q", path)
	}

	var payload struct {
		ChatID string `json:"chat_id"`
		Text   string `json:"text"`
	}
	if err := json.Unmarshal(captured, &payload); err != nil {
		t.Fatalf("payload 不是合法 JSON: %v", err)
	}
	if payload.ChatID != "42" {
		t.Fatalf("chat_id = %q", payload.ChatID)
	}
	if !strings.Contains(payload.Text, "标题") || !strings.Contains(payload.Text, "正文") {
		t.Fatalf("text = %q", payload.Text)
	}
}

func TestTelegram_DisableNotification(t *testing.T) {
	var captured []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	withTelegramBase(t, srv.URL)

	cfg, _ := json.Marshal(TelegramConfig{BotToken: "t", ChatID: "1", DisableNotification: true})
	if err := (&telegramNotifier{}).Send(t.Context(), cfg, Message{Body: "x"}); err != nil {
		t.Fatalf("发送应成功: %v", err)
	}
	var payload map[string]any
	_ = json.Unmarshal(captured, &payload)
	if payload["disable_notification"] != true {
		t.Fatalf("应带 disable_notification: %s", captured)
	}
}

// 4xx 与业务 ok=false 都应是永久错误；429 与 5xx 可重试。
func TestTelegram_ErrorClassification(t *testing.T) {
	cases := []struct {
		name      string
		status    int
		body      string
		permanent bool
	}{
		{"429", http.StatusTooManyRequests, `{"ok":false,"description":"Too Many Requests"}`, false},
		{"500", http.StatusInternalServerError, `{"ok":false}`, false},
		{"400", http.StatusBadRequest, `{"ok":false,"description":"Bad Request: chat not found"}`, true},
		{"200 ok=false", http.StatusOK, `{"ok":false,"error_code":403,"description":"bot was blocked"}`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			withTelegramBase(t, srv.URL)

			cfg, _ := json.Marshal(TelegramConfig{BotToken: "t", ChatID: "1"})
			err := (&telegramNotifier{}).Send(t.Context(), cfg, Message{Body: "x"})
			if err == nil {
				t.Fatal("应返回错误")
			}
			if got := IsPermanent(err); got != tc.permanent {
				t.Fatalf("IsPermanent = %v，期望 %v", got, tc.permanent)
			}
		})
	}
}

func TestTelegram_MissingConfig(t *testing.T) {
	cfg, _ := json.Marshal(TelegramConfig{ChatID: "1"})
	if err := (&telegramNotifier{}).Send(t.Context(), cfg, Message{Body: "x"}); err == nil || !IsPermanent(err) {
		t.Fatalf("缺少 token 应为永久错误，得到 %v", err)
	}
	cfg, _ = json.Marshal(TelegramConfig{BotToken: "t"})
	if err := (&telegramNotifier{}).Send(t.Context(), cfg, Message{Body: "x"}); err == nil || !IsPermanent(err) {
		t.Fatalf("缺少 chat_id 应为永久错误，得到 %v", err)
	}
}

// ==================== smtp_test.go ====================

func TestResolveSMTPSecurityMode(t *testing.T) {
	cases := []struct {
		name string
		cfg  SMTPConfig
		want string
	}{
		{name: "显式直连 TLS", cfg: SMTPConfig{SecurityMode: "implicit_tls"}, want: "implicit_tls"},
		{name: "显式 STARTTLS", cfg: SMTPConfig{SecurityMode: "starttls"}, want: "starttls"},
		{name: "显式明文", cfg: SMTPConfig{SecurityMode: "plain"}, want: "plain"},
		{name: "默认 STARTTLS", cfg: SMTPConfig{}, want: "starttls"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveSMTPSecurityMode(tc.cfg)
			if err != nil {
				t.Fatalf("resolveSMTPSecurityMode 失败: %v", err)
			}
			if got != tc.want {
				t.Fatalf("want %s, got %s", tc.want, got)
			}
		})
	}
}

func TestResolveSMTPSecurityModeRejectsUnknownMode(t *testing.T) {
	_, err := resolveSMTPSecurityMode(SMTPConfig{SecurityMode: "bad-mode"})
	if err == nil {
		t.Fatal("未知安全模式应报错")
	}
}

func TestSMTPPlain465DialErrorHintsImplicitTLSMismatch(t *testing.T) {
	cfg, err := json.Marshal(SMTPConfig{
		Host:         "127.0.0.1",
		Port:         465,
		FromAddr:     "sender@example.com",
		To:           []string{"receiver@example.com"},
		SecurityMode: "plain",
	})
	if err != nil {
		t.Fatalf("构造配置失败: %v", err)
	}

	err = (&smtpNotifier{}).Send(context.Background(), cfg, Message{Subject: "s", Body: "b"})
	if err == nil {
		t.Fatal("期望连接失败")
	}
	if !strings.Contains(err.Error(), "465 端口通常需要直连 TLS") {
		t.Fatalf("错误提示未包含协议修复建议: %v", err)
	}
}

func TestSMTPMissingRequiredFieldsIsPermanent(t *testing.T) {
	cfg, err := json.Marshal(SMTPConfig{Host: "smtp.example.com"})
	if err != nil {
		t.Fatalf("构造配置失败: %v", err)
	}

	err = (&smtpNotifier{}).Send(context.Background(), cfg, Message{})
	if err == nil {
		t.Fatal("期望缺少必填字段时报错")
	}
	if !IsPermanent(err) {
		t.Fatalf("缺少必填字段应为永久错误，实际: %v", err)
	}
}

// ==================== smtp_mime_test.go ====================

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
		{"默认 starttls", SMTPConfig{}, "starttls"},
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
