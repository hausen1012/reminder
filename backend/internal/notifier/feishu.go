// 飞书群自定义机器人通道。
package notifier

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/reminder/backend/internal/textutil"
)

// FeishuConfig 是飞书机器人的配置。
type FeishuConfig struct {
	WebhookURL string `json:"webhook_url"`
	Secret     string `json:"secret_enc"` // 启用签名校验时必填；传入时已是明文
	AtAll      bool   `json:"at_all,omitempty"`
}

type feishuNotifier struct{}

func (n *feishuNotifier) Type() string { return "feishu" }

func (n *feishuNotifier) Send(ctx context.Context, configJSON []byte, msg Message) error {
	var cfg FeishuConfig
	if err := json.Unmarshal(configJSON, &cfg); err != nil {
		return Permanent(fmt.Errorf("解析飞书配置失败: %w", err))
	}
	slog.Debug("飞书开始发送", "subject", msg.Subject, "body_len", len(msg.Body), "url", textutil.RedactURLString(cfg.WebhookURL))
	if cfg.WebhookURL == "" {
		return Permanent(fmt.Errorf("飞书 webhook_url 未配置"))
	}

	res, err := postJSON(ctx, "飞书", cfg.WebhookURL, buildFeishuPayload(cfg, msg), nil)
	if err != nil {
		return err
	}
	if err := classifyStatus("飞书", res); err != nil {
		return err
	}
	return checkBusinessCode("飞书", res, 0)
}

func buildFeishuPayload(cfg FeishuConfig, msg Message) map[string]any {
	// 飞书自定义机器人只发送纯文本：markdown 与 html 都先降级
	text := plainTextWithSubject(msg)
	if cfg.AtAll {
		text = `<at user_id="all">所有人</at> ` + text
	}
	payload := map[string]any{
		"msg_type": "text",
		"content":  map[string]any{"text": text},
	}
	if cfg.Secret != "" {
		ts, sign := signFeishu(cfg.Secret, time.Now())
		payload["timestamp"] = strconv.FormatInt(ts, 10)
		payload["sign"] = sign
	}
	return payload
}

// signFeishu 计算飞书签名：以 timestamp+"\n"+secret 为密钥对空字符串做 HMAC-SHA256。
func signFeishu(secret string, now time.Time) (int64, string) {
	ts := now.Unix()
	key := strconv.FormatInt(ts, 10) + "\n" + secret
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte{})
	return ts, base64.StdEncoding.EncodeToString(mac.Sum(nil))
}
