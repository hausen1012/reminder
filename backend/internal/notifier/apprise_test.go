package notifier

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

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
