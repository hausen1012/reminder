package llm

import (
	"context"
	"errors"
	"net"
	"net/url"
	"time"

	"github.com/anthropics/anthropic-sdk-go/v2"
)

// HTTPStatusError 是厂商返回的非 2xx 响应；带状态码是为了让重试策略区分临时与永久失败。
type HTTPStatusError struct {
	Status  int
	Message string
	// Body 是上游响应体的截断副本，只进日志不进用户可见错误。
	Body string
}

// Error 返回格式化错误描述。
func (e *HTTPStatusError) Error() string { return e.Message }

// 重试退避参数：第 n 次重试等待 base * 2^(n-1)，上限 max。
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

// RetryObserver 在一次重试真正开始等待之前被调用，用于把重试进度透传给前端。
type RetryObserver func(attempt, max int, delay time.Duration, err error)

// ChatWithRetry 调用 Provider，并在可重试错误上按指数退避重试。
//
// maxRetries 是「首次失败之后」的重试次数：0 表示只请求一次。
func ChatWithRetry(ctx context.Context, p Provider, req Request, maxRetries int) (*Reply, error) {
	return ChatWithRetryNotify(ctx, p, req, maxRetries, nil)
}

// ChatWithRetryNotify 与 ChatWithRetry 相同，但在每次重试前回调 obs（同步执行，必须立即返回）。
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

// RetryReason 将可重试错误转换为前端提示文案。
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
