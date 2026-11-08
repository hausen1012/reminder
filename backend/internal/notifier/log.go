// log 通道：把消息输出到服务端日志，供开发调试，无需配置。
package notifier

import (
	"context"
	"log/slog"
	"time"
)

// logNotifier 把消息写到服务端日志。
//
// Type 与 ChannelService 的类型校验、前端「日志输出」选项保持一致（都是 "log"）。
// 这是唯一允许输出完整正文的通道——它的用途就是打日志，正文里的 confirm_url 也一样。
type logNotifier struct{}

func (n *logNotifier) Type() string { return "log" }

func (n *logNotifier) Send(_ context.Context, _ []byte, msg Message) error {
	body := msg.Body
	switch msg.Format {
	case "html":
		body = StripHTML(msg.Body)
	case "markdown":
		body = StripMarkdown(msg.Body)
	}
	// 走 slog，保证受日志级别与文件输出控制
	slog.Info("通知(日志通道)", "at", time.Now().Format(time.RFC3339),
		"subject", msg.Subject, "format", msg.Format, "body", body)
	return nil
}
