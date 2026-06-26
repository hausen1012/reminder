package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// WeComConfig 是企业微信群机器人的配置。
type WeComConfig struct {
	WebhookURL    string   `json:"webhook_url"` // 完整的 https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=xxx
	MsgType       string   `json:"msg_type"`    // text | markdown，默认 text
	MentionedList []string `json:"mentioned_list,omitempty"`
	MentionedMobileList []string `json:"mentioned_mobile_list,omitempty"`
}

type wecomNotifier struct{}

func (n *wecomNotifier) Type() string { return "wecom" }

func (n *wecomNotifier) Send(ctx context.Context, configJSON []byte, msg Message) error {
	var cfg WeComConfig
	if err := json.Unmarshal(configJSON, &cfg); err != nil {
		return Permanent(fmt.Errorf("解析企微配置失败: %w", err))
	}
	slog.Info("开始发送", "subject", msg.Subject, "body_len", len(msg.Body), "format", msg.Format)
	if cfg.WebhookURL == "" {
		return Permanent(fmt.Errorf("企微 webhook_url 未配置"))
	}
	// 注意：不再使用 cfg.MsgType，改为使用 msg.Format

	payload := buildWeComPayload(cfg, msg)
	body, _ := json.Marshal(payload)

	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, cfg.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return Permanent(fmt.Errorf("构造请求失败: %w", err))
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("请求企微失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode >= 500 {
		return fmt.Errorf("企微 5xx %d: %s", resp.StatusCode, respBody)
	}
	if resp.StatusCode >= 400 {
		return Permanent(fmt.Errorf("企微 %d: %s", resp.StatusCode, respBody))
	}

	var br struct {
		Errcode int    `json:"errcode"`
		Errmsg  string `json:"errmsg"`
	}
	if err := json.Unmarshal(respBody, &br); err != nil {
		return fmt.Errorf("解析企微响应失败: %w (body=%s)", err, respBody)
	}
	if br.Errcode != 0 {
		return Permanent(fmt.Errorf("企微业务错误 errcode=%d errmsg=%s", br.Errcode, br.Errmsg))
	}
	return nil
}

func buildWeComPayload(cfg WeComConfig, msg Message) map[string]any {
	switch msg.Format {
	case "markdown":
		return map[string]any{
			"msgtype":  "markdown",
			"markdown": map[string]any{"content": msg.Body},
		}
	default:
		body := msg.Body
		if msg.Format == "html" {
			body = StripHTML(msg.Body)
		}
		content := body
		if msg.Subject != "" {
			content = msg.Subject + "\n" + body
		}
		text := map[string]any{"content": content}
		if len(cfg.MentionedList) > 0 {
			text["mentioned_list"] = cfg.MentionedList
		}
		if len(cfg.MentionedMobileList) > 0 {
			text["mentioned_mobile_list"] = cfg.MentionedMobileList
		}
		return map[string]any{
			"msgtype": "text",
			"text":    text,
		}
	}
}
