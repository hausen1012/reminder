package llm

import (
	"context"
	"errors"
	"net"
	"net/url"
	"time"

	"github.com/anthropics/anthropic-sdk-go/v2"
)

// HTTPStatusError 是 OpenAI 兼容厂商返回的非 2xx 响应。
//
// 单独定义类型（而不是只拼错误字符串）是为了让重试策略能按状态码判断：
// 限流（429）与 5xx 值得重试，密钥错（401）、参数错（400）重试多少次都没用。
type HTTPStatusError struct {
	Status  int
	Message string
	// Body 是上游响应体的截断副本，只进日志不进用户可见错误。
	//
	// 中转站经常把真实原因（例如「inference exceeds tpm/rpm limit」）
	// 放在响应体里而不是 message 字段，排错时这一段最关键。
	Body string
}

// Error 返回给上层看的错误文本（保持与旧实现的格式一致）。
func (e *HTTPStatusError) Error() string { return e.Message }

// 重试退避参数：第 n 次重试等待 base * 2^(n-1)，上限 max。
//
// 故意取得比较短：默认 10 次重试的最坏等待约 15 秒，
// 面板请求的超时是 120 秒，留足单次请求本身的耗时。
//
// 用变量而不是常量，便于测试里调小以避免真实等待。
var (
	retryBaseDelay = 200 * time.Millisecond
	retryMaxDelay  = 2 * time.Second
)

// retryBackoff 返回第 attempt 次重试（从 1 开始）前的等待时长。
func retryBackoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := retryBaseDelay
	for i := 1; i < attempt && delay < retryMaxDelay; i++ {
		delay *= 2
	}
	if delay > retryMaxDelay {
		delay = retryMaxDelay
	}
	return delay
}

// RetryObserver 在一次重试真正开始等待之前被调用。
//
// attempt 是本次重试的序号（从 1 开始），max 是配置的最大重试次数，
// delay 是接下来要等待的退避时长，err 是触发重试的原始错误。
// 上层用它把「正在重试」的进度透传给前端，避免用户对着一个转圈图标干等。
type RetryObserver func(attempt, max int, delay time.Duration, err error)

// ChatWithRetry 调用 Provider，并在可重试错误上按指数退避重试。
//
// maxRetries 是「首次失败之后」的重试次数：0 表示只请求一次。
// ctx 被取消或已超时时立即停止，不再发起下一次尝试。
func ChatWithRetry(ctx context.Context, p Provider, req Request, maxRetries int) (*Reply, error) {
	return ChatWithRetryNotify(ctx, p, req, maxRetries, nil)
}

// ChatWithRetryNotify 与 ChatWithRetry 相同，但在每次重试前回调 obs。
//
// obs 在调用方的 goroutine 里同步执行，实现必须立即返回：
// 它阻塞多久，重试就被推迟多久。
func ChatWithRetryNotify(ctx context.Context, p Provider, req Request, maxRetries int, obs RetryObserver) (*Reply, error) {
	if maxRetries < 0 {
		maxRetries = 0
	}
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		reply, err := p.Chat(ctx, req)
		if err == nil {
			return reply, nil
		}
		lastErr = err
		if attempt == maxRetries || !IsRetryable(err) || ctx.Err() != nil {
			break
		}
		delay := retryBackoff(attempt + 1)
		if obs != nil {
			obs(attempt+1, maxRetries, delay, err)
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, lastErr
		case <-timer.C:
		}
	}
	return nil, lastErr
}

// RetryReason 把可重试错误翻译成一句人话，供「正在重试」的进度提示使用。
//
// 面板上显示「正在重试 (2/10)」时，用户最想知道的是「为什么在等」：
// 限流通常几秒后就好了，网络错误可能需要检查本机网络。
func RetryReason(err error) string {
	if err == nil {
		return "请求失败"
	}
	var httpErr *HTTPStatusError
	if errors.As(err, &httpErr) {
		return reasonByStatus(httpErr.Status)
	}
	var apiErr *anthropic.Error
	if errors.As(err, &apiErr) {
		return reasonByStatus(apiErr.StatusCode)
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		if urlErr.Timeout() {
			return "请求超时"
		}
		return "网络连接失败"
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "请求超时"
	}
	return "服务暂时不可用"
}

// reasonByStatus 按 HTTP 状态码给出简短原因。
func reasonByStatus(status int) string {
	switch {
	case status == 429:
		return "服务繁忙（限流）"
	case status == 408:
		return "请求超时"
	case status >= 500:
		return "服务端错误"
	default:
		return "请求失败"
	}
}

// IsRetryable 判断错误是否值得重试。
//
// 可重试：网络层错误（连接失败、超时）、限流 429、请求超时 408、服务端 5xx。
// 不可重试：参数错误、鉴权失败等 4xx，以及被主动取消的请求。
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return false
	}

	var httpErr *HTTPStatusError
	if errors.As(err, &httpErr) {
		return retryableStatus(httpErr.Status)
	}

	var apiErr *anthropic.Error
	if errors.As(err, &apiErr) {
		return retryableStatus(apiErr.StatusCode)
	}

	// 网络层错误：连接被拒绝、连接重置、读超时等，换一次未必还失败。
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout()
	}
	return false
}

// retryableStatus 判断 HTTP 状态码是否属于临时性失败。
func retryableStatus(status int) bool {
	if status == 408 || status == 429 {
		return true
	}
	return status >= 500
}
