// log 通道：将通知输出到服务器控制台，用于开发和测试。
// 无需任何配置，不调用外部服务。
package notifier

import (
	"context"
	"fmt"
	"log"
	"time"
)

type logNotifier struct{}

func (n *logNotifier) Type() string { return "log" }

func (n *logNotifier) Send(_ context.Context, _ []byte, msg Message) error {
	log.Printf(
		"[log-notifier] %s | subject=%s | body=%s",
		time.Now().Format(time.RFC3339),
		msg.Subject,
		msg.Body,
	)
	fmt.Printf(
		"=== LOG NOTIFICATION ===\nTime: %s\nSubject: %s\nBody:\n%s\n========================\n",
		time.Now().Format(time.RFC3339),
		msg.Subject,
		msg.Body,
	)
	return nil
}