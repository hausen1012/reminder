// 飞书群自定义机器人。
//
// 请求体上限 20 KB；支持签名校验（timestamp + "\n" + secret 作为 HMAC-SHA256 密钥，
// 对空字符串求签名后 base64）。
package notifier

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"
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
	slog.Debug("飞书开始发送", "subject", msg.Subject, "body_len", len(msg.Body), "url", redactURLString(cfg.WebhookURL))
	if cfg.WebhookURL == "" {
		return Permanent(fmt.Errorf("飞书 webhook_url 未配置"))
	}

	payload := buildFeishuPayload(cfg, msg)
	body, _ := json.Marshal(payload)

	res, err := doRequest(ctx, http.MethodPost, cfg.WebhookURL,
		map[string]string{"Content-Type": "application/json; charset=utf-8"}, body)
	if err != nil {
		return err
	}
	if err := classifyStatus("飞书", res); err != nil {
		return err
	}

	var br struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(res.Body, &br); err != nil {
		return fmt.Errorf("解析飞书响应失败: %w (body=%s)", err, res.Body)
	}
	if br.Code != 0 {
		return Permanent(fmt.Errorf("飞书业务错误 code=%d msg=%s", br.Code, br.Msg))
	}
	return nil
}

func buildFeishuPayload(cfg FeishuConfig, msg Message) map[string]any {
	text := msg.Body
	switch msg.Format {
	case "html":
		text = StripHTML(msg.Body)
	case "markdown":
		// 只发送纯文本消息，markdown 标记降级去掉
		text = StripMarkdown(msg.Body)
	}
	if msg.Subject != "" {
		text = msg.Subject + "\n" + text
	}
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
