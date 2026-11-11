package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/reminder/backend/internal/logger"
)

// ==================== provider_test.go ====================

// OpenAI 兼容实现：请求体形状与工具调用解析。
func TestOpenAIProvider_Chat_SendsToolsAndParsesToolCalls(t *testing.T) {
	var gotBody map[string]any
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("路径 = %q，期望 /v1/chat/completions", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &gotBody); err != nil {
			t.Fatalf("请求体不是合法 JSON: %v", err)
		}
		_, _ = w.Write([]byte(`{
			"choices":[{
				"message":{
					"role":"assistant",
					"content":"先看看通道",
					"tool_calls":[{"id":"call_1","type":"function","function":{"name":"list_channels","arguments":"{\"x\":1}"}}]
				},
				"finish_reason":"tool_calls"
			}]
		}`))
	}))
	defer srv.Close()

	p := NewOpenAIProvider(srv.URL, "sk-test", "deepseek-chat")
	reply, err := p.Chat(context.Background(), Request{
		System: "系统提示",
		Turns:  []Turn{{Role: "user", Text: "明天早上九点提醒我"}},
		Tools: []Tool{{
			Name:        "list_channels",
			Description: "列出通道",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		}},
		MaxTokens: 512,
	})
	if err != nil {
		t.Fatalf("Chat 失败: %v", err)
	}

	if gotAuth != "Bearer sk-test" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if gotBody["model"] != "deepseek-chat" {
		t.Fatalf("model = %v", gotBody["model"])
	}
	msgs, _ := gotBody["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("消息数 = %d，期望 system + user", len(msgs))
	}
	if gotBody["tool_choice"] != "auto" {
		t.Fatalf("tool_choice = %v", gotBody["tool_choice"])
	}

	if reply.Text != "先看看通道" {
		t.Fatalf("reply.Text = %q", reply.Text)
	}
	if len(reply.ToolCalls) != 1 {
		t.Fatalf("工具调用数 = %d", len(reply.ToolCalls))
	}
	if reply.ToolCalls[0].Name != "list_channels" || string(reply.ToolCalls[0].Input) != `{"x":1}` {
		t.Fatalf("工具调用解析错误: %+v", reply.ToolCalls[0])
	}
}

// 工具结果要展开成多条 role=tool 消息，assistant 轮带 tool_calls。
func TestOpenAIProvider_ExpandsToolResults(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"完成"}}]}`))
	}))
	defer srv.Close()

	p := NewOpenAIProvider(srv.URL, "", "local-model")
	_, err := p.Chat(context.Background(), Request{
		Turns: []Turn{
			{Role: "user", Text: "看看通道"},
			{Role: "assistant", ToolCalls: []ToolCall{{ID: "c1", Name: "list_channels", Input: json.RawMessage(`{}`)}}},
			{Role: "user", ToolResults: []ToolResult{{CallID: "c1", Name: "list_channels", Content: `{"channels":[]}`}}},
		},
		MaxTokens: 256,
	})
	if err != nil {
		t.Fatalf("Chat 失败: %v", err)
	}
	msgs, _ := gotBody["messages"].([]any)
	if len(msgs) != 3 {
		t.Fatalf("消息数 = %d，期望 3（user/assistant/tool）", len(msgs))
	}
	asst, _ := msgs[1].(map[string]any)
	if asst["role"] != "assistant" || asst["tool_calls"] == nil {
		t.Fatalf("assistant 消息缺少 tool_calls: %v", asst)
	}
	toolMsg, _ := msgs[2].(map[string]any)
	if toolMsg["role"] != "tool" || toolMsg["tool_call_id"] != "c1" {
		t.Fatalf("tool 消息不正确: %v", toolMsg)
	}
}

// 4xx 应返回带对端消息的错误，而不是空错误。
func TestOpenAIProvider_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid api key","type":"authentication_error"}}`))
	}))
	defer srv.Close()

	p := NewOpenAIProvider(srv.URL, "bad", "m")
	_, err := p.Chat(context.Background(), Request{Turns: []Turn{{Role: "user", Text: "hi"}}})
	if err == nil {
		t.Fatal("应返回错误")
	}
	if !strings.Contains(err.Error(), "invalid api key") {
		t.Fatalf("错误信息未透出对端消息: %v", err)
	}
}

// base_url 的各种写法都应落到 /v1/chat/completions。
func TestNewOpenAIProvider_NormalizesBaseURL(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", "https://api.openai.com/v1"},
		{"https://api.deepseek.com", "https://api.deepseek.com/v1"},
		{"https://api.deepseek.com/", "https://api.deepseek.com/v1"},
		{"https://x.example/v1", "https://x.example/v1"},
		{"http://localhost:11434/v1/", "http://localhost:11434/v1"},
	}
	for _, c := range cases {
		if got := NewOpenAIProvider(c.in, "", "m").baseURL; got != c.want {
			t.Errorf("NewOpenAIProvider(%q).baseURL = %q，期望 %q", c.in, got, c.want)
		}
	}
}

// 工具调用参数为空时补成 {}，避免下游 JSON 解析失败。
func TestOpenAIProvider_EmptyArguments(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"tool_calls":[{"id":"c1","type":"function","function":{"name":"list_channels","arguments":""}}]}}]}`))
	}))
	defer srv.Close()

	p := NewOpenAIProvider(srv.URL, "", "m")
	reply, err := p.Chat(context.Background(), Request{Turns: []Turn{{Role: "user", Text: "x"}}})
	if err != nil {
		t.Fatalf("Chat 失败: %v", err)
	}
	if string(reply.ToolCalls[0].Input) != "{}" {
		t.Fatalf("空参数应补成 {}，实际 %q", reply.ToolCalls[0].Input)
	}
}

// requiredOf 从 schema 中取 required。
func TestRequiredOf(t *testing.T) {
	if got := requiredOf(map[string]any{"required": []any{"a", "b", 1}}); len(got) != 2 || got[0] != "a" {
		t.Fatalf("requiredOf 解析错误: %v", got)
	}
	if got := requiredOf(map[string]any{}); got != nil {
		t.Fatalf("无 required 应返回 nil，实际 %v", got)
	}
}

// ==================== errors_test.go ====================

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

// ==================== retry_test.go ====================

// scriptProvider 按调用次数返回预置结果，用于验证重试次数。
type scriptProvider struct {
	calls int
	errs  []error
	reply *Reply
}

func (p *scriptProvider) Chat(_ context.Context, _ Request) (*Reply, error) {
	p.calls++
	if len(p.errs) > 0 {
		err := p.errs[0]
		p.errs = p.errs[1:]
		if err != nil {
			return nil, err
		}
	}
	if p.reply == nil {
		return &Reply{Text: "ok"}, nil
	}
	return p.reply, nil
}

// 测试里不要真的等待指数退避。
func shrinkRetryDelays(t *testing.T) {
	t.Helper()
	oldBase, oldMax := retryBaseDelay, retryMaxDelay
	retryBaseDelay = time.Millisecond
	retryMaxDelay = 2 * time.Millisecond
	t.Cleanup(func() { retryBaseDelay, retryMaxDelay = oldBase, oldMax })
}

// 临时性错误应重试到成功为止。
func TestChatWithRetry_SucceedsAfterTransientErrors(t *testing.T) {
	shrinkRetryDelays(t)
	p := &scriptProvider{errs: []error{
		&HTTPStatusError{Status: 429, Message: "rate limited"},
		&HTTPStatusError{Status: 503, Message: "unavailable"},
		nil,
	}}

	reply, err := ChatWithRetry(context.Background(), p, Request{}, 5)
	if err != nil {
		t.Fatalf("应最终成功，实际 %v", err)
	}
	if reply.Text != "ok" {
		t.Fatalf("reply = %q", reply.Text)
	}
	if p.calls != 3 {
		t.Fatalf("调用次数 = %d，期望 3", p.calls)
	}
}

// 重试次数用尽后返回最后一次错误，总调用次数 = 1 + maxRetries。
func TestChatWithRetry_ExhaustsRetries(t *testing.T) {
	shrinkRetryDelays(t)
	p := &scriptProvider{errs: nil}
	// 每次都失败：用 errs 填满 + 让后续调用也报错
	for i := 0; i < 20; i++ {
		p.errs = append(p.errs, &HTTPStatusError{Status: 500, Message: "boom"})
	}

	_, err := ChatWithRetry(context.Background(), p, Request{}, 3)
	if err == nil {
		t.Fatal("应返回错误")
	}
	if p.calls != 4 {
		t.Fatalf("调用次数 = %d，期望 4（1 次 + 3 次重试）", p.calls)
	}
}

// maxRetries=0 表示只请求一次。
func TestChatWithRetry_ZeroRetries(t *testing.T) {
	p := &scriptProvider{errs: []error{&HTTPStatusError{Status: 500, Message: "boom"}}}
	if _, err := ChatWithRetry(context.Background(), p, Request{}, 0); err == nil {
		t.Fatal("应返回错误")
	}
	if p.calls != 1 {
		t.Fatalf("调用次数 = %d，期望 1", p.calls)
	}
}

// 不可重试的错误（鉴权、参数）应立即返回，不浪费时间重试。
func TestChatWithRetry_NotRetryable(t *testing.T) {
	shrinkRetryDelays(t)
	p := &scriptProvider{errs: []error{
		&HTTPStatusError{Status: 401, Message: "invalid api key"},
		nil,
	}}

	_, err := ChatWithRetry(context.Background(), p, Request{}, 10)
	if err == nil {
		t.Fatal("应返回错误")
	}
	if p.calls != 1 {
		t.Fatalf("401 不应重试，调用次数 = %d", p.calls)
	}
}

// ctx 取消后不再发起新的尝试。
func TestChatWithRetry_StopsOnCanceledContext(t *testing.T) {
	shrinkRetryDelays(t)
	p := &scriptProvider{}
	for i := 0; i < 10; i++ {
		p.errs = append(p.errs, &HTTPStatusError{Status: 500, Message: "boom"})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := ChatWithRetry(ctx, p, Request{}, 10); err == nil {
		t.Fatal("应返回错误")
	}
	if p.calls > 1 {
		t.Fatalf("ctx 已取消仍重试，调用次数 = %d", p.calls)
	}
}

// IsRetryable 的状态码与错误类型判定。
func TestIsRetryable(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"429", &HTTPStatusError{Status: 429}, true},
		{"408", &HTTPStatusError{Status: 408}, true},
		{"500", &HTTPStatusError{Status: 500}, true},
		{"502", &HTTPStatusError{Status: 502}, true},
		{"400", &HTTPStatusError{Status: 400}, false},
		{"401", &HTTPStatusError{Status: 401}, false},
		{"404", &HTTPStatusError{Status: 404}, false},
		{"anthropic 529", &anthropic.Error{StatusCode: 529}, true},
		{"anthropic 400", &anthropic.Error{StatusCode: 400}, false},
		{"context canceled", context.Canceled, false},
		{"url error", &url.Error{Op: "Post", URL: "http://x", Err: errors.New("refused")}, true},
		{"plain error", errors.New("something"), false},
	}
	for _, c := range cases {
		if got := IsRetryable(c.err); got != c.want {
			t.Errorf("%s: IsRetryable = %v，期望 %v", c.name, got, c.want)
		}
	}
	// net 超时错误算可重试
	var netErr net.Error = &net.DNSError{IsTimeout: true}
	if !IsRetryable(netErr) {
		t.Error("网络超时应可重试")
	}
}

// 退避时长应递增并受上限约束。
func TestRetryBackoff(t *testing.T) {
	oldBase, oldMax := retryBaseDelay, retryMaxDelay
	retryBaseDelay = 100 * time.Millisecond
	retryMaxDelay = 500 * time.Millisecond
	t.Cleanup(func() { retryBaseDelay, retryMaxDelay = oldBase, oldMax })

	if got := retryBackoff(1); got != 100*time.Millisecond {
		t.Errorf("第 1 次 = %v", got)
	}
	if got := retryBackoff(2); got != 200*time.Millisecond {
		t.Errorf("第 2 次 = %v", got)
	}
	if got := retryBackoff(10); got != 500*time.Millisecond {
		t.Errorf("第 10 次应封顶 500ms，实际 %v", got)
	}
}

// OpenAI 兼容厂商的 5xx 响应应被识别为可重试，并能真实重试成功。
func TestOpenAIProvider_RetriesOn5xx(t *testing.T) {
	shrinkRetryDelays(t)
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&attempts, 1) < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":{"message":"overloaded"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"好了"}}]}`))
	}))
	defer srv.Close()

	p := NewOpenAIProvider(srv.URL, "", "m")
	reply, err := ChatWithRetry(context.Background(), p, Request{
		Turns: []Turn{{Role: "user", Text: "hi"}},
	}, 5)
	if err != nil {
		t.Fatalf("应重试后成功: %v", err)
	}
	if reply.Text != "好了" {
		t.Fatalf("reply = %q", reply.Text)
	}
	if got := atomic.LoadInt32(&attempts); got != 3 {
		t.Fatalf("请求次数 = %d，期望 3", got)
	}
}

// 反向代理或网关返回非 JSON 的 HTML 502/503 页面时，同样应被识别为 HTTPStatusError 并重试。
func TestOpenAIProvider_RetriesOnHTML5xx(t *testing.T) {
	shrinkRetryDelays(t)
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&attempts, 1) < 2 {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`<html><body>502 Bad Gateway</body></html>`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"恢复正常"}}]}`))
	}))
	defer srv.Close()

	p := NewOpenAIProvider(srv.URL, "", "m")
	reply, err := ChatWithRetry(context.Background(), p, Request{
		Turns: []Turn{{Role: "user", Text: "hi"}},
	}, 3)
	if err != nil {
		t.Fatalf("应重试后成功: %v", err)
	}
	if reply.Text != "恢复正常" {
		t.Fatalf("reply = %q", reply.Text)
	}
	if got := atomic.LoadInt32(&attempts); got != 2 {
		t.Fatalf("请求次数 = %d，期望 2", got)
	}
}

// finish_reason=length 应标记为截断。
func TestOpenAIProvider_TruncatedFlag(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"说到一半"},"finish_reason":"length"}]}`))
	}))
	defer srv.Close()

	reply, err := NewOpenAIProvider(srv.URL, "", "m").Chat(context.Background(), Request{})
	if err != nil {
		t.Fatalf("Chat 失败: %v", err)
	}
	if !reply.Truncated {
		t.Fatal("finish_reason=length 应标记 Truncated")
	}
}

// RetryReason 把错误翻译成给用户看的一句话。
func TestRetryReason(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"nil", nil, "请求失败"},
		{"429", &HTTPStatusError{Status: 429}, "服务繁忙（限流）"},
		{"408", &HTTPStatusError{Status: 408}, "请求超时"},
		{"503", &HTTPStatusError{Status: 503}, "服务端错误"},
		{"400", &HTTPStatusError{Status: 400}, "请求失败"},
		{"anthropic 529", &anthropic.Error{StatusCode: 529}, "服务端错误"},
		{"url timeout", &url.Error{Op: "Post", Err: context.DeadlineExceeded}, "请求超时"},
		{"url refused", &url.Error{Op: "Post", Err: errors.New("refused")}, "网络连接失败"},
		{"plain", errors.New("x"), "服务暂时不可用"},
	}
	for _, c := range cases {
		if got := RetryReason(c.err); got != c.want {
			t.Errorf("%s: RetryReason = %q，期望 %q", c.name, got, c.want)
		}
	}
}

// 进度回调应在每次重试前被调用，且序号递增。
func TestChatWithRetryNotify_ReportsEachAttempt(t *testing.T) {
	shrinkRetryDelays(t)
	p := &scriptProvider{}
	for i := 0; i < 10; i++ {
		p.errs = append(p.errs, &HTTPStatusError{Status: 500, Message: "boom"})
	}

	var attempts []int
	var maxSeen int
	if _, err := ChatWithRetryNotify(context.Background(), p, Request{}, 2,
		func(attempt, max int, delay time.Duration, _ error) {
			attempts = append(attempts, attempt)
			maxSeen = max
			if delay <= 0 {
				t.Errorf("退避时长应为正数，实际 %v", delay)
			}
		}); err == nil {
		t.Fatal("应返回错误")
	}
	if len(attempts) != 2 || attempts[0] != 1 || attempts[1] != 2 {
		t.Fatalf("回调序号 = %v，期望 [1 2]", attempts)
	}
	if maxSeen != 2 {
		t.Fatalf("max = %d，期望 2", maxSeen)
	}
}
