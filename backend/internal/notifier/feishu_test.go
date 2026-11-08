package notifier

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
