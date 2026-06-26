// log 通道：将通知输出到服务器控制台，用于开发和测试。
// 无需任何配置，不调用外部服务。
package notifier

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

type logNotifier struct{}

func (n *logNotifier) Type() string { return "log/slog" }

func (n *logNotifier) Send(_ context.Context, _ []byte, msg Message) error {
	body := msg.Body
	switch msg.Format {
	case "html":
		body = StripHTML(msg.Body)
	case "markdown":
		body = StripMarkdown(msg.Body)
	}
	slog.Info("通知", "time", time.Now().Format(time.RFC3339), "subject", msg.Subject, "body", body)
	fmt.Printf(
		"=== LOG NOTIFICATION ===\nTime: %s\nSubject: %s\nBody:\n%s\n========================\n",
		time.Now().Format(time.RFC3339),
		msg.Subject,
		body,
	)
	return nil
}