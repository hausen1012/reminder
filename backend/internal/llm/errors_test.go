package llm

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/reminder/backend/internal/logger"
)

// captureSlog 把全局 slog 临时重定向到内存 buffer。
func captureSlog(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(old) })
	return buf
}

// 上游非 2xx 时，日志必须带状态码与响应体：
// 中转站的真实原因（limits / 余额）常只在 body 里，只记错误字符串等于没记。
func TestOpenAIProvider_LogsUpstreamErrorBody(t *testing.T) {
	logs := captureSlog(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"inference exceeds tpm/rpm limit"}}`))
	}))
	defer srv.Close()

	p := NewOpenAIProvider(srv.URL, "sk-test", "deepseek-chat")
	_, err := p.Chat(context.Background(), Request{Turns: []Turn{{Role: "user", Text: "hi"}}})
	if err == nil {
		t.Fatal("应返回错误")
	}

	out := logs.String()
	for _, want := range []string{
		"AI 请求返回错误",
		"provider=openai",
		"model=deepseek-chat",
		"status=429",
		"tpm/rpm limit",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("上游错误日志缺少 %q:\n%s", want, out)
		}
	}

	// 错误体里的原文同样要能通过类型取到
	var httpErr *HTTPStatusError
	if !errors.As(err, &httpErr) {
		t.Fatalf("错误类型应为 *HTTPStatusError，实际 %T", err)
	}
	if !strings.Contains(httpErr.Body, "tpm/rpm limit") {
		t.Fatalf("HTTPStatusError.Body 应保留上游原文，实际 %q", httpErr.Body)
	}
}

// 请求地址里的 key 不能进日志。
func TestOpenAIProvider_RedactsURLInLogs(t *testing.T) {
	logs := captureSlog(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":"boom"}}`))
	}))
	defer srv.Close()

	p := NewOpenAIProvider(srv.URL+"/v1?key=sk-secret", "sk-secret", "m")
	_, _ = p.Chat(context.Background(), Request{Turns: []Turn{{Role: "user", Text: "x"}}})

	if out := logs.String(); strings.Contains(out, "sk-secret") {
		t.Fatalf("日志不应包含 URL 里的密钥:\n%s", out)
	}
}

// 错误日志应自动带上请求的 request_id，能和面板请求对上。
func TestProvider_LogsCarryRequestID(t *testing.T) {
	logs := captureSlog(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":{"message":"upstream down"}}`))
	}))
	defer srv.Close()

	ctx := logger.ContextWithRequestID(context.Background(), "req-abc-123")
	p := NewOpenAIProvider(srv.URL, "", "m")
	_, _ = p.Chat(ctx, Request{Turns: []Turn{{Role: "user", Text: "x"}}})

	if out := logs.String(); !strings.Contains(out, "request_id=req-abc-123") {
		t.Fatalf("上游错误日志应带 request_id:\n%s", out)
	}
}

// ErrorAttrs 把 HTTPStatusError 展开成 status / error / body。
func TestErrorAttrs_HTTPStatusError(t *testing.T) {
	attrs := ErrorAttrs(&HTTPStatusError{Status: 429, Message: "限流", Body: `{"x":1}`})
	m := attrsToMap(attrs)

	if m["status"] != 429 {
		t.Fatalf("status = %v，期望 429", m["status"])
	}
	if m["error"] != "限流" {
		t.Fatalf("error = %v", m["error"])
	}
	if m["body"] != `{"x":1}` {
		t.Fatalf("body = %v", m["body"])
	}
}

// 普通错误只展开成 error，不凭空造字段。
func TestErrorAttrs_PlainError(t *testing.T) {
	attrs := ErrorAttrs(errors.New("网络断开"))
	m := attrsToMap(attrs)
	if m["error"] != "网络断开" {
		t.Fatalf("error = %v", m["error"])
	}
	if _, ok := m["status"]; ok {
		t.Fatalf("普通错误不应有 status: %v", m)
	}
	if ErrorAttrs(nil) != nil {
		t.Fatal("nil 错误应返回 nil")
	}
}

// ErrorAttrsWith 保留调用方字段并附加错误细节。
func TestErrorAttrsWith_KeepsCallerFields(t *testing.T) {
	m := attrsToMap(ErrorAttrsWith(errors.New("boom"), "round", 3, "user_message", "你好"))
	if m["round"] != 3 || m["user_message"] != "你好" {
		t.Fatalf("调用方字段丢失: %v", m)
	}
	if m["error"] != "boom" {
		t.Fatalf("error = %v", m["error"])
	}
}

// attrsToMap 把 key/value 切片转成 map，奇数长度时忽略末位。
//
// error 值统一转成文本：调用方关心的是「写了什么」，不是具体错误类型。
func attrsToMap(attrs []any) map[string]any {
	m := make(map[string]any, len(attrs)/2)
	for i := 0; i+1 < len(attrs); i += 2 {
		k, _ := attrs[i].(string)
		if err, ok := attrs[i+1].(error); ok {
			m[k] = err.Error()
			continue
		}
		m[k] = attrs[i+1]
	}
	return m
}
