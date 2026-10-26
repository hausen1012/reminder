package notifier

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
