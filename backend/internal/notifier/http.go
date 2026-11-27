// HTTP 通道请求工具：发送请求并读取响应，网络与超时错误返回可重试错误。
package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// sendTimeout 是单次外部请求的超时上限。
const sendTimeout = 10 * time.Second

// responseLimit 是读取响应体的上限，避免对端返回超大内容。
const responseLimit = 8192

type proxyContextKey struct{}

// WithProxyURL 将决议后的生效代理地址存入 context。
func WithProxyURL(ctx context.Context, proxyURL string) context.Context {
	return context.WithValue(ctx, proxyContextKey{}, proxyURL)
}

// ProxyURLFromContext 从 context 中取出代理地址。
func ProxyURLFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(proxyContextKey{}).(string); ok {
		return v
	}
	return ""
}

// ResolveProxy 决议最终生效的代理 URL：
// 1. channelProxy 为 direct（不区分大小写）-> 强制直连，返回 ""
// 2. channelProxy 不为空 -> 通道单独配置优先
// 3. 否则回退使用 globalProxy；若也为空则返回 ""（直连）
func ResolveProxy(channelProxy, globalProxy string) string {
	cp := strings.TrimSpace(channelProxy)
	if strings.EqualFold(cp, "direct") {
		return ""
	}
	if cp != "" {
		return cp
	}
	return strings.TrimSpace(globalProxy)
}

// ExtractChannelProxy 从通道的明文 JSON 配置中快速提取 proxy_url 字段。
func ExtractChannelProxy(configJSON []byte) string {
	if len(configJSON) == 0 {
		return ""
	}
	var c struct {
		ProxyURL string `json:"proxy_url"`
	}
	_ = json.Unmarshal(configJSON, &c)
	return strings.TrimSpace(c.ProxyURL)
}

var (
	proxyClientsMu sync.RWMutex
	proxyClients   = make(map[string]*http.Client)
)

// httpClientFor 返回当前 context 对应的 HTTP Client。
func httpClientFor(ctx context.Context) *http.Client {
	proxyURL := ProxyURLFromContext(ctx)
	if proxyURL == "" {
		return http.DefaultClient
	}

	proxyClientsMu.RLock()
	client, ok := proxyClients[proxyURL]
	proxyClientsMu.RUnlock()
	if ok {
		return client
	}

	proxyClientsMu.Lock()
	defer proxyClientsMu.Unlock()
	if client, ok := proxyClients[proxyURL]; ok {
		return client
	}

	u, err := url.Parse(proxyURL)
	if err != nil {
		slog.Warn("解析代理地址失败，回退默认客户端", "proxy_url", proxyURL, "error", err)
		return http.DefaultClient
	}

	transport := &http.Transport{
		Proxy:                 http.ProxyURL(u),
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	newClient := &http.Client{
		Transport: transport,
	}
	proxyClients[proxyURL] = newClient
	return newClient
}

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

	client := httpClientFor(reqCtx)
	resp, err := client.Do(req)
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

// checkBusinessCode 解析 IM 通道的业务响应码，非 okCode 视为永久错误。
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
