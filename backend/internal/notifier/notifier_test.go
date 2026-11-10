package notifier

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

// ==================== notifier_test.go ====================


func TestWebhook_POST_DefaultBody(t *testing.T) {
	var captured struct {
		body        string
		method      string
		contentType string
		auth        string
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		captured.body = string(body)
		captured.method = r.Method
		captured.contentType = r.Header.Get("Content-Type")
		captured.auth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg, _ := json.Marshal(WebhookConfig{
		Method:           "POST",
		URL:              srv.URL,
		AuthorizationEnc: "Bearer my-token",
	})
	n := &webhookNotifier{}
	err := n.Send(context.Background(), cfg, Message{Subject: "标题", Body: "正文"})
	if err != nil {
		t.Fatalf("send 失败: %v", err)
	}
	if captured.method != "POST" {
		t.Errorf("method = %s", captured.method)
	}
	if !strings.Contains(captured.contentType, "application/json") {
		t.Errorf("content-type = %s", captured.contentType)
	}
	if captured.auth != "Bearer my-token" {
		t.Errorf("auth = %s", captured.auth)
	}
	if !strings.Contains(captured.body, `"subject":"标题"`) || !strings.Contains(captured.body, `"body":"正文"`) {
		t.Errorf("body = %s", captured.body)
	}
}

func TestWebhook_POST_TemplateBody(t *testing.T) {
	var captured string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		captured = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg, _ := json.Marshal(WebhookConfig{
		Method:       "POST",
		URL:          srv.URL,
		BodyTemplate: `{"title":"{{title}}","msg":"{{content}}"}`,
	})
	n := &webhookNotifier{}
	err := n.Send(context.Background(), cfg, Message{
		Vars: map[string]string{"title": "提醒标题", "content": "提醒正文"},
	})
	if err != nil {
		t.Fatalf("send 失败: %v", err)
	}
	// 逐字段渲染后走 json.Marshal，map 键顺序不确定，故比较解析后的结构而非原始字符串
	var got map[string]string
	if err := json.Unmarshal([]byte(captured), &got); err != nil {
		t.Fatalf("响应体不是合法 JSON: %v, body=%s", err, captured)
	}
	want := map[string]string{"title": "提醒标题", "msg": "提醒正文"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("body = %s, want %v", captured, want)
	}
}

func TestWebhook_GET_QueryTemplate(t *testing.T) {
	var capturedQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg, _ := json.Marshal(WebhookConfig{
		Method:        "GET",
		URL:           srv.URL,
		QueryTemplate: map[string]string{"text": "{{title}}-{{content}}"},
	})
	n := &webhookNotifier{}
	err := n.Send(context.Background(), cfg, Message{Vars: map[string]string{"title": "x", "content": "y"}})
	if err != nil {
		t.Fatalf("send 失败: %v", err)
	}
	if !strings.Contains(capturedQuery, "text=x-y") {
		t.Errorf("query = %s", capturedQuery)
	}
}

func TestWebhook_4xx_IsPermanent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("bad"))
	}))
	defer srv.Close()
	cfg, _ := json.Marshal(WebhookConfig{Method: "POST", URL: srv.URL})
	err := (&webhookNotifier{}).Send(context.Background(), cfg, Message{Body: "x"})
	if err == nil || !IsPermanent(err) {
		t.Fatalf("期望永久错误，得到 %v", err)
	}
}

func TestWebhook_5xx_IsRetryable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	cfg, _ := json.Marshal(WebhookConfig{Method: "POST", URL: srv.URL})
	err := (&webhookNotifier{}).Send(context.Background(), cfg, Message{Body: "x"})
	if err == nil || IsPermanent(err) {
		t.Fatalf("期望可重试错误，得到 %v", err)
	}
}

func TestDingTalk_Errcode_IsPermanent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"errcode":310000,"errmsg":"keywords not in content"}`))
	}))
	defer srv.Close()
	cfg, _ := json.Marshal(DingTalkConfig{WebhookURL: srv.URL})
	err := (&dingtalkNotifier{}).Send(context.Background(), cfg, Message{Body: "x"})
	if err == nil || !IsPermanent(err) {
		t.Fatalf("errcode != 0 应是永久错误，得到 %v", err)
	}
}

func TestDingTalk_Success(t *testing.T) {
	var captured map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&captured)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
	}))
	defer srv.Close()
	cfg, _ := json.Marshal(DingTalkConfig{WebhookURL: srv.URL})
	err := (&dingtalkNotifier{}).Send(context.Background(), cfg, Message{Subject: "S", Body: "B"})
	if err != nil {
		t.Fatalf("send 失败: %v", err)
	}
	if captured["msgtype"] != "text" {
		t.Errorf("msgtype = %v", captured["msgtype"])
	}
}

func TestWeCom_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
	}))
	defer srv.Close()
	cfg, _ := json.Marshal(WeComConfig{WebhookURL: srv.URL})
	err := (&wecomNotifier{}).Send(context.Background(), cfg, Message{Body: "hi"})
	if err != nil {
		t.Fatalf("send 失败: %v", err)
	}
}

func TestPermanentWrapping(t *testing.T) {
	if IsPermanent(nil) {
		t.Error("nil 不应被认为是永久错误")
	}
	if Permanent(nil) != nil {
		t.Error("Permanent(nil) 应返回 nil")
	}
}

// ==================== registry_test.go ====================


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

// ==================== template_test.go ====================


func TestRender_BasicSubstitution(t *testing.T) {
	got := Render("你好 {{name}}，今天 {{date}}", map[string]string{
		"name": "张三",
		"date": "2026-06-05",
	})
	want := "你好 张三，今天 2026-06-05"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRender_UnknownPlaceholderPreserved(t *testing.T) {
	got := Render("{{title}} - {{missing}}", map[string]string{"title": "提醒"})
	want := "提醒 - {{missing}}"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRender_NoRecursion(t *testing.T) {
	// vars 里的值即使再含占位符也不会二次展开
	got := Render("{{x}}", map[string]string{"x": "{{y}}", "y": "should-not-appear"})
	want := "{{y}}"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRender_WhitespaceAround(t *testing.T) {
	got := Render("{{  name  }} 你好", map[string]string{"name": "李四"})
	want := "李四 你好"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRender_DotInName(t *testing.T) {
	got := Render("{{user.name}}", map[string]string{"user.name": "王五"})
	if got != "王五" {
		t.Errorf("got %q, want %q", got, "王五")
	}
}

func TestRender_EmptyTemplate(t *testing.T) {
	if got := Render("", map[string]string{"a": "b"}); got != "" {
		t.Errorf("got %q, want empty string", got)
	}
}

func TestRender_NilVars(t *testing.T) {
	got := Render("{{x}} y", nil)
	if got != "{{x}} y" {
		t.Errorf("got %q, want %q", got, "{{x}} y")
	}
}

func TestRenderMap(t *testing.T) {
	in := map[string]string{
		"key1": "{{a}}",
		"key2": "static",
	}
	out := RenderMap(in, map[string]string{"a": "value"})
	if out["key1"] != "value" || out["key2"] != "static" {
		t.Errorf("got %+v", out)
	}
}

func TestHasPlaceholder(t *testing.T) {
	if !HasPlaceholder("{{x}}") {
		t.Error("expect HasPlaceholder true for {{x}}")
	}
	if HasPlaceholder("no placeholder") {
		t.Error("expect HasPlaceholder false for plain text")
	}
	if HasPlaceholder("{{ }}") {
		t.Error("expect HasPlaceholder false for malformed placeholder")
	}
}

// ==================== redact_test.go ====================


// URL 里的 query 取值一律抹掉，只保留参数名。
// 钉钉 / 企微的凭据就在 query 里（access_token、key、sign），不能落日志。
func TestRedactURLString(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "钉钉 access_token 与 sign",
			in:   "https://oapi.dingtalk.com/robot/send?access_token=SECRET&sign=SIG",
			want: "https://oapi.dingtalk.com/robot/send?access_token=%2A%2A%2A&sign=%2A%2A%2A",
		},
		{
			name: "企微 key",
			in:   "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=abc-123",
			want: "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=%2A%2A%2A",
		},
		{
			name: "无 query 时原样返回",
			in:   "https://example.com/hook",
			want: "https://example.com/hook",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := redactURLString(tc.in)
			if got != tc.want {
				t.Fatalf("redactURLString(%q) = %q，期望 %q", tc.in, got, tc.want)
			}
			// 关键断言：原文里的凭据值不能出现在输出里
			if u, err := url.Parse(tc.in); err == nil {
				for _, vals := range u.Query() {
					for _, v := range vals {
						if len(v) >= 4 && strings.Contains(got, v) {
							t.Fatalf("输出里仍包含凭据值 %q: %q", v, got)
						}
					}
				}
			}
		})
	}
}

// userinfo 也是凭据，必须抹掉。
func TestRedactURL_UserInfo(t *testing.T) {
	u, err := url.Parse("https://user:pass@example.com/hook")
	if err != nil {
		t.Fatal(err)
	}
	got := redactURL(u)
	if strings.Contains(got, "pass") {
		t.Fatalf("输出里仍包含密码: %q", got)
	}
}

// headerKeys 只返回名字，不返回值。
func TestHeaderKeys(t *testing.T) {
	h := http.Header{}
	h.Set("Authorization", "Bearer SECRET")
	h.Set("Content-Type", "application/json")

	keys := headerKeys(h)
	if len(keys) != 2 {
		t.Fatalf("keys = %v，期望 2 个", keys)
	}
	if keys[0] != "Authorization" || keys[1] != "Content-Type" {
		t.Fatalf("keys = %v，期望按字母序排列", keys)
	}
	for _, k := range keys {
		if strings.Contains(k, "SECRET") {
			t.Fatalf("key 里混入了 header 取值: %q", k)
		}
	}
}

// ==================== bodylimit_test.go ====================


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
