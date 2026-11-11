// Telegram Bot API 通道。
package notifier

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
)

// telegramAPIBase 是 Bot API 根地址；测试可替换为本地服务。
var telegramAPIBase = "https://api.telegram.org"

// TelegramConfig 是 Telegram 通道的配置。
type TelegramConfig struct {
	BotToken            string `json:"bot_token_enc"` // 传入时已是明文
	ChatID              string `json:"chat_id"`
	DisableNotification bool   `json:"disable_notification,omitempty"`
}

type telegramNotifier struct{}

func (n *telegramNotifier) Type() string { return "telegram" }

func (n *telegramNotifier) Send(ctx context.Context, configJSON []byte, msg Message) error {
	var cfg TelegramConfig
	if err := json.Unmarshal(configJSON, &cfg); err != nil {
		return Permanent(fmt.Errorf("解析 Telegram 配置失败: %w", err))
	}
	// 只记录 chat_id 与正文长度，token 属于凭据
	slog.Debug("Telegram 开始发送", "chat_id", cfg.ChatID, "subject", msg.Subject, "body_len", len(msg.Body), "format", msg.Format)
	if cfg.BotToken == "" {
		return Permanent(fmt.Errorf("Telegram bot_token 未配置"))
	}
	if cfg.ChatID == "" {
		return Permanent(fmt.Errorf("Telegram chat_id 未配置"))
	}

	endpoint := telegramAPIBase + "/bot" + cfg.BotToken + "/sendMessage"
	res, err := postJSON(ctx, "Telegram", endpoint, buildTelegramPayload(cfg, msg), nil)
	if err != nil {
		return redactTokenErr(err, cfg.BotToken)
	}
	if err := classifyStatus("Telegram", res); err != nil {
		return err
	}

	var br struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
		ErrorCode   int    `json:"error_code"`
	}
	_ = json.Unmarshal(res.Body, &br)
	if !br.OK {
		return Permanent(fmt.Errorf("Telegram 业务错误 %d: %s（%s）", br.ErrorCode, br.Description, res.Body))
	}
	return nil
}

func buildTelegramPayload(cfg TelegramConfig, msg Message) map[string]any {
	payload := map[string]any{
		"chat_id": cfg.ChatID,
		"text":    plainTextWithSubject(msg),
	}
	if cfg.DisableNotification {
		payload["disable_notification"] = true
	}
	return payload
}

// redactTokenErr 把错误信息里可能出现的 bot token 替换掉。
func redactTokenErr(err error, token string) error {
	if token == "" {
		return err
	}
	return fmt.Errorf("%s", strings.ReplaceAll(err.Error(), token, "***"))
}
