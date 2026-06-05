package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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
			q.Set(k, Render(vTmpl, msg.Vars))
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
		var bodyBytes []byte
		if cfg.BodyTemplate != "" {
			bodyBytes = []byte(Render(cfg.BodyTemplate, msg.Vars))
		} else {
			// 缺省 body：{"subject": "...", "body": "..."}
			bodyBytes, _ = json.Marshal(map[string]string{"subject": msg.Subject, "body": msg.Body})
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

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("请求 Webhook 失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode >= 500 {
		return fmt.Errorf("Webhook 5xx %d: %s", resp.StatusCode, respBody)
	}
	if resp.StatusCode >= 400 {
		return Permanent(fmt.Errorf("Webhook %d: %s", resp.StatusCode, respBody))
	}
	return nil
}
