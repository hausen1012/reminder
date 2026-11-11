package notifier

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/reminder/backend/internal/textutil"
)

// webhookTimeout 是 Webhook 单次请求的超时：用户可能指向较慢的自建服务。
const webhookTimeout = 15 * time.Second

// WebhookConfig 是 Webhook 通道的配置。
//
// GET 按 query_template 拼 query string；POST 把 body_template 渲染后作为请求体。
type WebhookConfig struct {
	Method           string            `json:"method"` // GET | POST
	URL              string            `json:"url"`
	Headers          map[string]string `json:"headers,omitempty"`           // 普通 header
	AuthorizationEnc string            `json:"authorization_enc,omitempty"` // 可选，落库加密；传入时已是明文
	BodyTemplate     string            `json:"body_template,omitempty"`     // POST 时使用
	QueryTemplate    map[string]string `json:"query_template,omitempty"`    // GET 时使用，value 内允许占位符
	ContentType      string            `json:"content_type,omitempty"`      // 默认 application/json
}

type webhookNotifier struct{}

func (n *webhookNotifier) Type() string { return "webhook" }

func (n *webhookNotifier) Send(ctx context.Context, configJSON []byte, msg Message) error {
	var cfg WebhookConfig
	if err := json.Unmarshal(configJSON, &cfg); err != nil {
		return Permanent(fmt.Errorf("解析 Webhook 配置失败: %w", err))
	}
	slog.Debug("Webhook 开始发送", "method", cfg.Method, "url", textutil.RedactURLString(cfg.URL), "subject", msg.Subject, "body_len", len(msg.Body))
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

	// 提前校验 URL 格式，避免无效 URL 触发无意义重试
	parsedURL, urlErr := url.Parse(cfg.URL)
	if urlErr != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		return Permanent(fmt.Errorf("Webhook url 不合法: %s（需要 http:// 或 https:// 开头）", cfg.URL))
	}

	headers := map[string]string{}
	for k, v := range cfg.Headers {
		headers[k] = v
	}
	if cfg.AuthorizationEnc != "" {
		headers["Authorization"] = cfg.AuthorizationEnc
	}

	var (
		res *httpResult
		err error
	)
	switch method {
	case http.MethodGet:
		q := parsedURL.Query()
		for k, vTmpl := range cfg.QueryTemplate {
			rendered := Render(vTmpl, msg.Vars)
			// 渲染结果可能带 {{confirm_url}} 之类的凭据，只记长度
			slog.Debug("Webhook query 参数已渲染", "key", k, "value_len", len(rendered))
			q.Set(k, rendered)
		}
		// 没显式配置 query_template 时，把 subject/body 当成默认参数附上
		if len(cfg.QueryTemplate) == 0 {
			if msg.Subject != "" {
				q.Set("subject", msg.Subject)
			}
			q.Set("body", msg.Body)
		}
		parsedURL.RawQuery = q.Encode()
		// 只记 header 名不记取值，Authorization 是解密后的凭据
		slog.Debug("Webhook 发送请求", "method", http.MethodGet, "url", textutil.RedactURL(parsedURL), "header_keys", headerKeyList(headers))
		res, err = doRequestTimeout(ctx, webhookTimeout, http.MethodGet, parsedURL.String(), headers, nil)
	case http.MethodPost:
		bodyBytes, buildErr := buildPostBody(cfg.BodyTemplate, msg)
		if buildErr != nil {
			return Permanent(fmt.Errorf("构造 Webhook body 失败: %w", buildErr))
		}
		ct := cfg.ContentType
		if ct == "" {
			ct = "application/json; charset=utf-8"
		}
		headers["Content-Type"] = ct
		slog.Debug("Webhook 发送请求", "method", http.MethodPost, "url", textutil.RedactURLString(cfg.URL), "header_keys", headerKeyList(headers))
		res, err = doRequestTimeout(ctx, webhookTimeout, http.MethodPost, cfg.URL, headers, bodyBytes)
	}
	if err != nil {
		// 这里只返回错误，dispatch 会带重试轮次与通道信息统一记录
		return fmt.Errorf("请求 Webhook 失败: %w", err)
	}
	slog.Debug("Webhook 响应", "status", res.Status, "body_len", len(res.Body), "body", string(res.Body))
	return classifyStatus("Webhook", res)
}

// buildPostBody 构造 POST 请求体，支持 JSON 模板与纯文本模板渲染。
func buildPostBody(bodyTemplate string, msg Message) ([]byte, error) {
	if bodyTemplate == "" {
		bodyBytes, _ := json.Marshal(map[string]string{"subject": msg.Subject, "body": msg.Body})
		slog.Debug("缺省 body", "body_len", len(bodyBytes))
		return bodyBytes, nil
	}

	// 尝试解析为 JSON 树，逐字符串值渲染占位符
	var parsed interface{}
	if err := json.Unmarshal([]byte(bodyTemplate), &parsed); err == nil {
		walkAndRender(&parsed, msg.Vars)
		bodyBytes, _ := json.Marshal(parsed)
		slog.Debug("JSON 结构逐字段渲染", "body_len", len(bodyBytes))
		return bodyBytes, nil
	}

	// 不是合法 JSON，回退到整串渲染
	rendered := Render(bodyTemplate, msg.Vars)
	slog.Debug("BodyTemplate 非 JSON，整串渲染", "rendered_len", len(rendered))

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
