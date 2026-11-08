package notifier

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
