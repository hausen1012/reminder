// log 通道：将通知输出到服务器控制台，用于开发和测试。
// 无需任何配置，不调用外部服务。
package notifier

import (
	"context"
	"log/slog"
	"time"
)

// logNotifier 把消息写到服务端日志，便于本地调试通道配置。
//
// Type 必须返回 "log"：ChannelService 只允许 "log" 这种类型，
// 前端「日志输出」选项提交的也是 "log"，三者需保持一致。
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
	// 统一走 slog，避免 fmt.Printf 绕过日志级别与文件写入
	slog.Info("通知(日志通道)", "at", time.Now().Format(time.RFC3339),
		"subject", msg.Subject, "format", msg.Format, "body", body)
	return nil
}
