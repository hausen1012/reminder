package notifier

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
	want := `{"title":"提醒标题","msg":"提醒正文"}`
	if captured != want {
		t.Errorf("body = %s, want %s", captured, want)
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
	cfg, _ := json.Marshal(DingTalkConfig{WebhookURL: srv.URL, MsgType: "text"})
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
	cfg, _ := json.Marshal(WeComConfig{WebhookURL: srv.URL, MsgType: "text"})
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
