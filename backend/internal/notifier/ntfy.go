// ntfy 通道：通过 HTTP Header 传递标题、优先级与标签。
package notifier

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/reminder/backend/internal/textutil"
)

// DefaultNtfyServer 是 ntfy 官方服务端地址。
const DefaultNtfyServer = "https://ntfy.sh"

// NtfyConfig 是 ntfy 通道的配置。
type NtfyConfig struct {
	ServerURL string `json:"server_url,omitempty"`
	Topic     string `json:"topic"`
	Token     string `json:"token_enc,omitempty"` // 传入时已是明文
	Priority  string `json:"priority,omitempty"`
	Tags      string `json:"tags,omitempty"` // 逗号分隔
}

type ntfyNotifier struct{}

func (n *ntfyNotifier) Type() string { return "ntfy" }

func (n *ntfyNotifier) Send(ctx context.Context, configJSON []byte, msg Message) error {
	var cfg NtfyConfig
	if err := json.Unmarshal(configJSON, &cfg); err != nil {
		return Permanent(fmt.Errorf("解析 ntfy 配置失败: %w", err))
	}
	server := strings.TrimRight(strings.TrimSpace(cfg.ServerURL), "/")
	if server == "" {
		server = DefaultNtfyServer
	}
	topic := strings.TrimSpace(cfg.Topic)
	slog.Debug("ntfy 开始发送", "server", textutil.RedactURLString(server), "topic", topic, "subject", msg.Subject, "body_len", len(msg.Body))
	if topic == "" {
		return Permanent(fmt.Errorf("ntfy topic 未配置"))
	}

	endpoint := server + "/" + topic
	headers := map[string]string{}
	body := buildNtfyBody(msg)
	if msg.Subject != "" {
		headers["Title"] = msg.Subject
	}
	if cfg.Priority != "" && cfg.Priority != "default" {
		headers["Priority"] = cfg.Priority
	}
	if tags := normalizeCSV(cfg.Tags); tags != "" {
		headers["Tags"] = tags
	}
	if cfg.Token != "" {
		headers["Authorization"] = "Bearer " + cfg.Token
	}

	res, err := doRequest(ctx, http.MethodPost, endpoint, headers, []byte(body))
	if err != nil {
		return err
	}
	return classifyStatus("ntfy", res)
}

// buildNtfyBody 返回消息正文：ntfy 客户端按纯文本展示，富文本先降级。
func buildNtfyBody(msg Message) string {
	return plainText(msg)
}

// normalizeCSV 去掉逗号分隔项的多余空白；空项被丢弃。
func normalizeCSV(s string) string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, ",")
}
