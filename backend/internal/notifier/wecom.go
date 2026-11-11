package notifier

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/reminder/backend/internal/textutil"
)

// WeComConfig 是企业微信群机器人的配置。
type WeComConfig struct {
	WebhookURL          string   `json:"webhook_url"` // 完整的 https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=xxx
	MentionedList       []string `json:"mentioned_list,omitempty"`
	MentionedMobileList []string `json:"mentioned_mobile_list,omitempty"`
}

type wecomNotifier struct{}

func (n *wecomNotifier) Type() string { return "wecom" }

func (n *wecomNotifier) Send(ctx context.Context, configJSON []byte, msg Message) error {
	var cfg WeComConfig
	if err := json.Unmarshal(configJSON, &cfg); err != nil {
		return Permanent(fmt.Errorf("解析企微配置失败: %w", err))
	}
	slog.Debug("企微开始发送", "subject", msg.Subject, "body_len", len(msg.Body), "format", msg.Format, "url", textutil.RedactURLString(cfg.WebhookURL))
	if cfg.WebhookURL == "" {
		return Permanent(fmt.Errorf("企微 webhook_url 未配置"))
	}

	res, err := postJSON(ctx, "企微", cfg.WebhookURL, buildWeComPayload(cfg, msg), nil)
	if err != nil {
		return err
	}
	if err := classifyStatus("企微", res); err != nil {
		return err
	}
	return checkBusinessCode("企微", res, 0)
}

func buildWeComPayload(cfg WeComConfig, msg Message) map[string]any {
	switch msg.Format {
	case "markdown":
		return map[string]any{
			"msgtype":  "markdown",
			"markdown": map[string]any{"content": msg.Body},
		}
	default:
		// text 原文；html 去标签后按纯文本发送
		text := map[string]any{"content": plainTextWithSubject(msg)}
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
