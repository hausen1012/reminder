package notifier

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

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
