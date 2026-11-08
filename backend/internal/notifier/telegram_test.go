package notifier

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
