package notifier

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
