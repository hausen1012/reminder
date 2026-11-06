package llm

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go/v2"
)

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
