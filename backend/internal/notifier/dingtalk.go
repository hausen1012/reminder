package notifier

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"time"

	"github.com/reminder/backend/internal/textutil"
)

// DingTalkConfig 是钉钉机器人 Webhook 的配置。
type DingTalkConfig struct {
	WebhookURL string   `json:"webhook_url"`
	Secret     string   `json:"secret_enc"` // 可选，启用了加签时必填；传入时已是明文
	AtMobiles  []string `json:"at_mobiles,omitempty"`
	AtAll      bool     `json:"at_all,omitempty"`
}

type dingtalkNotifier struct{}

func (n *dingtalkNotifier) Type() string { return "dingtalk" }

func (n *dingtalkNotifier) Send(ctx context.Context, configJSON []byte, msg Message) error {
	var cfg DingTalkConfig
	if err := json.Unmarshal(configJSON, &cfg); err != nil {
		return Permanent(fmt.Errorf("解析钉钉配置失败: %w", err))
	}
	slog.Debug("钉钉开始发送", "subject", msg.Subject, "body_len", len(msg.Body), "format", msg.Format, "url", textutil.RedactURLString(cfg.WebhookURL))
	if cfg.WebhookURL == "" {
		return Permanent(fmt.Errorf("钉钉 webhook_url 未配置"))
	}

	endpoint := cfg.WebhookURL
	if cfg.Secret != "" {
		signed, err := signDingTalk(endpoint, cfg.Secret)
		if err != nil {
			return Permanent(fmt.Errorf("钉钉签名失败: %w", err))
		}
		endpoint = signed
	}

	res, err := postJSON(ctx, "钉钉", endpoint, buildDingTalkPayload(cfg, msg), nil)
	if err != nil {
		return err
	}
	if err := classifyStatus("钉钉", res); err != nil {
		return err
	}
	return checkBusinessCode("钉钉", res, 0)
}

func signDingTalk(rawURL, secret string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	ts := fmt.Sprintf("%d", time.Now().UnixMilli())
	stringToSign := ts + "\n" + secret
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(stringToSign))
	sign := base64.StdEncoding.EncodeToString(h.Sum(nil))

	q := u.Query()
	q.Set("timestamp", ts)
	q.Set("sign", sign)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func buildDingTalkPayload(cfg DingTalkConfig, msg Message) map[string]any {
	switch msg.Format {
	case "markdown":
		title := msg.Subject
		if title == "" {
			title = "提醒"
		}
		p := map[string]any{
			"msgtype": "markdown",
			"markdown": map[string]any{
				"title": title,
				"text":  msg.Body,
			},
		}
		if cfg.AtAll || len(cfg.AtMobiles) > 0 {
			p["at"] = map[string]any{"atMobiles": cfg.AtMobiles, "isAtAll": cfg.AtAll}
		}
		return p
	default:
		// text 原文；html 去标签后按纯文本发送
		p := map[string]any{
			"msgtype": "text",
			"text":    map[string]any{"content": plainTextWithSubject(msg)},
		}
		if cfg.AtAll || len(cfg.AtMobiles) > 0 {
			p["at"] = map[string]any{"atMobiles": cfg.AtMobiles, "isAtAll": cfg.AtAll}
		}
		return p
	}
}
