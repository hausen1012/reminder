// 外部 HTTP 通道共用的请求工具。
//
// 只负责发请求与读取响应，状态码含义由各通道自行判断；
// 传输层错误（网络、超时）返回可重试错误。
package notifier

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// sendTimeout 是单次外部请求的超时上限。
const sendTimeout = 10 * time.Second

// responseLimit 是读取响应体的上限，避免对端返回超大内容。
const responseLimit = 8192

// httpResult 是一次请求的结果。
type httpResult struct {
	Status int
	Body   []byte
}

// doRequest 发送 HTTP 请求并读取响应。
//
// 构造请求失败视为永久错误；网络或超时错误不做包装，交给 dispatch 重试。
func doRequest(ctx context.Context, method, endpoint string, headers map[string]string, body []byte) (*httpResult, error) {
	reqCtx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()

	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(reqCtx, method, endpoint, reader)
	if err != nil {
		return nil, Permanent(fmt.Errorf("构造请求失败: %w", err))
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, responseLimit))
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}
	return &httpResult{Status: resp.StatusCode, Body: respBody}, nil
}

// classifyStatus 把 HTTP 状态码映射为错误：5xx 可重试，4xx 中限流可重试、其余永久错误。
func classifyStatus(name string, res *httpResult) error {
	switch {
	case res.Status >= 200 && res.Status < 300:
		return nil
	case res.Status == http.StatusTooManyRequests:
		return fmt.Errorf("%s 限流 %d: %s", name, res.Status, res.Body)
	case res.Status >= 500:
		return fmt.Errorf("%s %d: %s", name, res.Status, res.Body)
	default:
		return Permanent(fmt.Errorf("%s %d: %s", name, res.Status, res.Body))
	}
}
