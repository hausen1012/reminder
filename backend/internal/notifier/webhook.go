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
	"time"
)

// WebhookConfig 是通用 Webhook 通道的配置。
//
// Method=GET：subject/body/vars 一律按 query_template 拼到 query string；
// Method=POST：直接把 body_template 渲染后作为请求体发送，Content-Type 默认 application/json。
type WebhookConfig struct {
	Method        string            `json:"method"` // GET | POST
	URL           string            `json:"url"`
	Headers       map[string]string `json:"headers,omitempty"`        // 普通 header
	AuthorizationEnc string         `json:"authorization_enc,omitempty"` // 可选，落库加密；传入时已是明文
	BodyTemplate  string            `json:"body_template,omitempty"`  // POST 时使用
	QueryTemplate map[string]string `json:"query_template,omitempty"` // GET 时使用，value 内允许占位符
	ContentType   string            `json:"content_type,omitempty"`   // 默认 application/json
}

type webhookNotifier struct{}

func (n *webhookNotifier) Type() string { return "webhook" }

func (n *webhookNotifier) Send(ctx context.Context, configJSON []byte, msg Message) error {
	var cfg WebhookConfig
	if err := json.Unmarshal(configJSON, &cfg); err != nil {
		return Permanent(fmt.Errorf("解析 Webhook 配置失败: %w", err))
	}
	slog.Info("开始发送", "method", cfg.Method, "url", cfg.URL, "subject", msg.Subject, "body_len", len(msg.Body))
	if cfg.URL == "" {
		return Permanent(fmt.Errorf("Webhook url 未配置"))
	}
	method := strings.ToUpper(strings.TrimSpace(cfg.Method))
	if method == "" {
		method = http.MethodPost
	}
	if method != http.MethodGet && method != http.MethodPost {
		return Permanent(fmt.Errorf("Webhook method 仅支持 GET/POST，收到 %s", method))
	}

	reqCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	// 提前校验 URL 格式，避免无效 URL 触发无意义重试
	parsedURL, urlErr := url.Parse(cfg.URL)
	if urlErr != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		return Permanent(fmt.Errorf("Webhook url 不合法: %s（需要 http:// 或 https:// 开头）", cfg.URL))
	}

	var req *http.Request
	var err error
	switch method {
	case http.MethodGet:
		u, err := url.Parse(cfg.URL)
		if err != nil {
			return Permanent(fmt.Errorf("Webhook url 不合法: %w", err))
		}
		q := u.Query()
		for k, vTmpl := range cfg.QueryTemplate {
			rendered := Render(vTmpl, msg.Vars)
			slog.Info("query param", "key", k, "value", rendered)
			q.Set(k, rendered)
		}
		// 没显式配置 query_template 时，把 subject/body 当成默认参数附上
		if len(cfg.QueryTemplate) == 0 {
			if msg.Subject != "" {
				q.Set("subject", msg.Subject)
			}
			q.Set("body", msg.Body)
		}
		u.RawQuery = q.Encode()
		req, err = http.NewRequestWithContext(reqCtx, http.MethodGet, u.String(), nil)
		if err != nil {
			return Permanent(fmt.Errorf("构造 GET 请求失败: %w", err))
		}
	case http.MethodPost:
		bodyBytes, err := buildPostBody(cfg.BodyTemplate, msg)
		if err != nil {
			return Permanent(fmt.Errorf("构造 Webhook body 失败: %w", err))
		}
		req, err = http.NewRequestWithContext(reqCtx, http.MethodPost, cfg.URL, bytes.NewReader(bodyBytes))
		if err != nil {
			return Permanent(fmt.Errorf("构造 POST 请求失败: %w", err))
		}
		ct := cfg.ContentType
		if ct == "" {
			ct = "application/json; charset=utf-8"
		}
		req.Header.Set("Content-Type", ct)
	}

	for k, v := range cfg.Headers {
		req.Header.Set(k, v)
	}
	if cfg.AuthorizationEnc != "" {
		req.Header.Set("Authorization", cfg.AuthorizationEnc)
	}

	slog.Info("发送请求", "method", req.Method, "url", req.URL.String(), "headers", req.Header)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.Info("请求失败", "error", err)
		return fmt.Errorf("请求 Webhook 失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	slog.Info("响应", "status", resp.StatusCode, "body", string(respBody))
	if resp.StatusCode >= 500 {
		return fmt.Errorf("Webhook 5xx %d: %s", resp.StatusCode, respBody)
	}
	if resp.StatusCode >= 400 {
		return Permanent(fmt.Errorf("Webhook %d: %s", resp.StatusCode, respBody))
	}
	return nil
}

// buildPostBody 构造 POST 请求体。
//
// 如果 BodyTemplate 是合法 JSON，先解析 JSON 树，再逐字符串值替换占位符，
// 最后 json.Marshal 确保特殊字符正确转义。
// 如果 BodyTemplate 不是 JSON 或为空，退回到整串渲染或默认 {subject, body} 结构。
func buildPostBody(bodyTemplate string, msg Message) ([]byte, error) {
	if bodyTemplate == "" {
		bodyBytes, _ := json.Marshal(map[string]string{"subject": msg.Subject, "body": msg.Body})
		slog.Info("缺省 body", "body", string(bodyBytes))
		return bodyBytes, nil
	}

	// 尝试解析为 JSON 树，逐字符串值渲染占位符
	var parsed interface{}
	if err := json.Unmarshal([]byte(bodyTemplate), &parsed); err == nil {
		walkAndRender(&parsed, msg.Vars)
		bodyBytes, _ := json.Marshal(parsed)
		slog.Info("JSON 结构逐字段渲染", "body", string(bodyBytes))
		return bodyBytes, nil
	}

	// 不是合法 JSON，回退到整串渲染
	rendered := Render(bodyTemplate, msg.Vars)
	slog.Info("BodyTemplate 非 JSON，整串渲染", "rendered", rendered)

	// 尝试把渲染结果当 JSON 发送：如果渲染后成了合法 JSON 就用结构化方式序列化
	var again interface{}
	if err := json.Unmarshal([]byte(rendered), &again); err == nil {
		bodyBytes, _ := json.Marshal(again)
		return bodyBytes, nil
	}

	return []byte(rendered), nil
}

// walkAndRender 递归遍历 JSON 树，对每个字符串值执行占位符渲染。
func walkAndRender(node *interface{}, vars map[string]string) {
	switch val := (*node).(type) {
	case string:
		rendered := Render(val, vars)
		*node = rendered
	case map[string]interface{}:
		for k, v := range val {
			walkAndRender(&v, vars)
			val[k] = v
		}
	case []interface{}:
		for i, v := range val {
			walkAndRender(&v, vars)
			val[i] = v
		}
	}
}