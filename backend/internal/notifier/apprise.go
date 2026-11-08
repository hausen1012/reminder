// Apprise 通道：把消息交给 Apprise API 服务转发。
//
// 两种模式：
//   - 状态化：配置里只填 key，服务端使用该 KEY 保存的目标列表发送；
//   - 无状态：配置里填 urls，请求体带上目标列表，key 仅用于鉴权（可留空）。
//
// 请求成功后服务端返回 200；424 表示没有任何目标发送成功，属于永久错误。
package notifier

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
)

// AppriseConfig 是 Apprise 通道的配置。
type AppriseConfig struct {
	APIURL string `json:"api_url"`
	Key    string `json:"key_enc,omitempty"` // 传入时已是明文
	URLs   string `json:"urls,omitempty"`    // 多行，每行一个 Apprise URL
	Type   string `json:"type,omitempty"`    // info | success | warning | failure
}

type appriseNotifier struct{}

func (n *appriseNotifier) Type() string { return "apprise" }

func (n *appriseNotifier) Send(ctx context.Context, configJSON []byte, msg Message) error {
	var cfg AppriseConfig
	if err := json.Unmarshal(configJSON, &cfg); err != nil {
		return Permanent(fmt.Errorf("解析 Apprise 配置失败: %w", err))
	}
	apiURL := strings.TrimRight(strings.TrimSpace(cfg.APIURL), "/")
	slog.Debug("Apprise 开始发送", "url", redactURLString(apiURL), "subject", msg.Subject, "body_len", len(msg.Body))
	if apiURL == "" {
		return Permanent(fmt.Errorf("Apprise api_url 未配置"))
	}

	urls := normalizeLines(cfg.URLs)
	key := strings.TrimSpace(cfg.Key)
	if key == "" && len(urls) == 0 {
		return Permanent(fmt.Errorf("Apprise 需要配置 key 或 urls 之一"))
	}

	endpoint := apiURL + "/notify"
	if key != "" {
		endpoint += "/" + key
	}

	body := buildApprisePayload(cfg, urls, msg)
	raw, _ := json.Marshal(body)

	res, err := doRequest(ctx, http.MethodPost, endpoint,
		map[string]string{"Content-Type": "application/json"}, raw)
	if err != nil {
		return err
	}

	switch res.Status {
	case http.StatusOK:
		return nil
	case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return fmt.Errorf("Apprise 暂时不可用 %d: %s", res.Status, res.Body)
	case http.StatusUnprocessableEntity, http.StatusFailedDependency:
		return Permanent(fmt.Errorf("Apprise 没有目标发送成功 %d: %s", res.Status, res.Body))
	case http.StatusInternalServerError:
		return fmt.Errorf("Apprise 5xx %d: %s", res.Status, res.Body)
	default:
		return Permanent(fmt.Errorf("Apprise %d: %s", res.Status, res.Body))
	}
}

func buildApprisePayload(cfg AppriseConfig, urls []string, msg Message) map[string]any {
	body := msg.Body
	switch msg.Format {
	case "html":
		body = StripHTML(msg.Body)
	case "markdown":
		body = StripMarkdown(msg.Body)
	}
	payload := map[string]any{
		"title": msg.Subject,
		"body":  body,
		"type":  normalizeAppriseType(cfg.Type),
	}
	if len(urls) > 0 {
		// 带显式 urls 时按无状态模式发送
		payload["urls"] = strings.Join(urls, ",")
	}
	return payload
}

// normalizeAppriseType 只允许 Apprise 支持的四种消息类型。
func normalizeAppriseType(t string) string {
	switch t {
	case "success", "warning", "failure":
		return t
	default:
		return "info"
	}
}

// normalizeLines 按行切分并去掉空行与首尾空白。
func normalizeLines(s string) []string {
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}
