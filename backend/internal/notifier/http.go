// 外部 HTTP 通道共用的请求工具。
//
// 只负责发请求与读取响应，状态码含义由各通道自行判断；
// 传输层错误（网络、超时）返回可重试错误。
package notifier

import (
	"bytes"
	"context"
	"encoding/json"
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
	return doRequestTimeout(ctx, sendTimeout, method, endpoint, headers, body)
}

// doRequestTimeout 与 doRequest 相同，但使用指定的单次请求超时。
func doRequestTimeout(ctx context.Context, timeout time.Duration, method, endpoint string, headers map[string]string, body []byte) (*httpResult, error) {
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
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

// postJSON 把 payload 序列化为 JSON 并 POST 到 endpoint；未指定 Content-Type 时补上 JSON 类型。
func postJSON(ctx context.Context, name, endpoint string, payload any, headers map[string]string) (*httpResult, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, Permanent(fmt.Errorf("序列化%s请求失败: %w", name, err))
	}
	if headers == nil {
		headers = map[string]string{}
	}
	if _, ok := headers["Content-Type"]; !ok {
		headers["Content-Type"] = "application/json; charset=utf-8"
	}
	return doRequest(ctx, http.MethodPost, endpoint, headers, body)
}

// classifyStatus 把 HTTP 状态码映射为错误：5xx 与 429 可重试，其余 4xx 为永久错误。
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

// checkBusinessCode 解析国内 IM 通道的「HTTP 200 + 业务码」响应：okCode 之外一律视为永久错误。
//
// 钉钉 / 企微用 errcode+errmsg，飞书用 code+msg，Bark 用 code+message，这里合并处理。
func checkBusinessCode(name string, res *httpResult, okCode int) error {
	var br struct {
		Errcode int    `json:"errcode"`
		Errmsg  string `json:"errmsg"`
		Code    int    `json:"code"`
		Msg     string `json:"msg"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(res.Body, &br); err != nil {
		return fmt.Errorf("解析%s响应失败: %w (body=%s)", name, err, res.Body)
	}
	code, msg := br.Errcode, br.Errmsg
	if code == 0 {
		code, msg = br.Code, br.Msg
	}
	if msg == "" {
		msg = br.Message
	}
	if code != okCode {
		return Permanent(fmt.Errorf("%s业务错误 code=%d msg=%s", name, code, msg))
	}
	return nil
}
