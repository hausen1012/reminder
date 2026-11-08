// ntfy 通道：发布消息到话题。
//
// 走 Header 形式（POST /{topic}），正文即消息体；标题、优先级、标签通过请求头传递。
// 访问令牌以 Bearer 形式发送，日志里不打印。
package notifier

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
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
	slog.Debug("ntfy 开始发送", "server", redactURLString(server), "topic", topic, "subject", msg.Subject, "body_len", len(msg.Body))
	if topic == "" {
		return Permanent(fmt.Errorf("ntfy topic 未配置"))
	}

	endpoint := server + "/" + topic
	headers := map[string]string{}
	body := buildNtfyBody(cfg, msg)
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

// buildNtfyBody 返回消息正文：html / markdown 降级为纯文本。
func buildNtfyBody(cfg NtfyConfig, msg Message) string {
	body := msg.Body
	switch msg.Format {
	case "html":
		body = StripHTML(msg.Body)
	case "markdown":
		// ntfy 客户端默认按纯文本展示，markdown 标记直接去掉
		body = StripMarkdown(msg.Body)
	}
	return body
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
