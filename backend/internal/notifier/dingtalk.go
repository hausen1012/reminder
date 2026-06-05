package notifier

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// DingTalkConfig 是钉钉机器人 Webhook 的配置。
type DingTalkConfig struct {
	WebhookURL string `json:"webhook_url"`
	Secret     string `json:"secret_enc"` // 可选，启用了加签时必填；传入时已是明文
	MsgType    string `json:"msg_type"`   // text | markdown，默认 text
	AtMobiles  []string `json:"at_mobiles,omitempty"`
	AtAll      bool   `json:"at_all,omitempty"`
}

type dingtalkNotifier struct{}

func (n *dingtalkNotifier) Type() string { return "dingtalk" }

func (n *dingtalkNotifier) Send(ctx context.Context, configJSON []byte, msg Message) error {
	var cfg DingTalkConfig
	if err := json.Unmarshal(configJSON, &cfg); err != nil {
		return Permanent(fmt.Errorf("解析钉钉配置失败: %w", err))
	}
	if cfg.WebhookURL == "" {
		return Permanent(fmt.Errorf("钉钉 webhook_url 未配置"))
	}
	if cfg.MsgType == "" {
		cfg.MsgType = "text"
	}

	endpoint := cfg.WebhookURL
	if cfg.Secret != "" {
		signed, err := signDingTalk(endpoint, cfg.Secret)
		if err != nil {
			return Permanent(fmt.Errorf("钉钉签名失败: %w", err))
		}
		endpoint = signed
	}

	payload := buildDingTalkPayload(cfg, msg)
	body, _ := json.Marshal(payload)

	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return Permanent(fmt.Errorf("构造请求失败: %w", err))
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("请求钉钉失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode >= 500 {
		return fmt.Errorf("钉钉 5xx %d: %s", resp.StatusCode, respBody)
	}
	if resp.StatusCode >= 400 {
		return Permanent(fmt.Errorf("钉钉 %d: %s", resp.StatusCode, respBody))
	}

	// 业务码解析
	var br struct {
		Errcode int    `json:"errcode"`
		Errmsg  string `json:"errmsg"`
	}
	if err := json.Unmarshal(respBody, &br); err != nil {
		return fmt.Errorf("解析钉钉响应失败: %w (body=%s)", err, respBody)
	}
	if br.Errcode != 0 {
		return Permanent(fmt.Errorf("钉钉业务错误 errcode=%d errmsg=%s", br.Errcode, br.Errmsg))
	}
	return nil
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
	switch cfg.MsgType {
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
		content := msg.Body
		if msg.Subject != "" {
			content = msg.Subject + "\n" + msg.Body
		}
		p := map[string]any{
			"msgtype": "text",
			"text":    map[string]any{"content": content},
		}
		if cfg.AtAll || len(cfg.AtMobiles) > 0 {
			p["at"] = map[string]any{"atMobiles": cfg.AtMobiles, "isAtAll": cfg.AtAll}
		}
		return p
	}
}
