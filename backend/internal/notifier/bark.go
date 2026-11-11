// Bark 通道（iOS 推送）。
package notifier

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/reminder/backend/internal/textutil"
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
	slog.Debug("Bark 开始发送", "server", textutil.RedactURLString(server), "subject", msg.Subject, "body_len", len(msg.Body))
	if cfg.DeviceKey == "" {
		return Permanent(fmt.Errorf("Bark device_key 未配置"))
	}
	if _, err := url.Parse(server); err != nil {
		return Permanent(fmt.Errorf("Bark server_url 非法: %w", err))
	}

	res, err := postJSON(ctx, "Bark", server+"/push", buildBarkPayload(cfg, msg), nil)
	if err != nil {
		return err
	}
	if err := classifyStatus("Bark", res); err != nil {
		return err
	}
	return checkBusinessCode("Bark", res, http.StatusOK)
}

func buildBarkPayload(cfg BarkConfig, msg Message) map[string]any {
	payload := map[string]any{
		"device_key": cfg.DeviceKey,
		"title":      msg.Subject,
		"body":       plainText(msg),
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
