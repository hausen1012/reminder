// Bark 通道：iOS 推送，支持官方与自建服务端。
//
// 请求形式为 POST {server}/push，JSON body 里带 device_key。
// Device Key 属于凭据，日志里只记录主机名。
package notifier

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
)

// DefaultBarkServer 是 Bark 官方服务端地址。
const DefaultBarkServer = "https://api.day.app"

// BarkConfig 是 Bark 通道的配置。
type BarkConfig struct {
	DeviceKey string `json:"device_key_enc"` // 传入时已是明文
	ServerURL string `json:"server_url,omitempty"`
	Group     string `json:"group,omitempty"`
	Sound     string `json:"sound,omitempty"`
	Icon      string `json:"icon,omitempty"`
}

type barkNotifier struct{}

func (n *barkNotifier) Type() string { return "bark" }

func (n *barkNotifier) Send(ctx context.Context, configJSON []byte, msg Message) error {
	var cfg BarkConfig
	if err := json.Unmarshal(configJSON, &cfg); err != nil {
		return Permanent(fmt.Errorf("解析 Bark 配置失败: %w", err))
	}
	server := strings.TrimRight(strings.TrimSpace(cfg.ServerURL), "/")
	if server == "" {
		server = DefaultBarkServer
	}
	slog.Debug("Bark 开始发送", "server", redactURLString(server), "subject", msg.Subject, "body_len", len(msg.Body))
	if cfg.DeviceKey == "" {
		return Permanent(fmt.Errorf("Bark device_key 未配置"))
	}
	if _, err := url.Parse(server); err != nil {
		return Permanent(fmt.Errorf("Bark server_url 非法: %w", err))
	}

	payload := buildBarkPayload(cfg, msg)
	body, _ := json.Marshal(payload)

	res, err := doRequest(ctx, http.MethodPost, server+"/push",
		map[string]string{"Content-Type": "application/json; charset=utf-8"}, body)
	if err != nil {
		return err
	}
	if err := classifyStatus("Bark", res); err != nil {
		return err
	}

	var br struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(res.Body, &br); err != nil {
		return fmt.Errorf("解析 Bark 响应失败: %w (body=%s)", err, res.Body)
	}
	if br.Code != 200 {
		return Permanent(fmt.Errorf("Bark 业务错误 code=%d message=%s", br.Code, br.Message))
	}
	return nil
}

func buildBarkPayload(cfg BarkConfig, msg Message) map[string]any {
	body := msg.Body
	switch msg.Format {
	case "html":
		body = StripHTML(msg.Body)
	case "markdown":
		body = StripMarkdown(msg.Body)
	}
	payload := map[string]any{
		"device_key": cfg.DeviceKey,
		"title":      msg.Subject,
		"body":       body,
	}
	if cfg.Group != "" {
		payload["group"] = cfg.Group
	}
	if cfg.Sound != "" {
		payload["sound"] = cfg.Sound
	}
	if cfg.Icon != "" {
		payload["icon"] = cfg.Icon
	}
	return payload
}
