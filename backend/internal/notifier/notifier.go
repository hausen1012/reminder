// Package notifier 定义通知通道的统一接口与各通道实现。
//
// 错误约定：nil 表示成功；包装 ErrPermanent 表示不应重试（鉴权失败、参数错误、对端业务码非 0）；
// 其他 error 表示可重试（网络、超时、5xx）。
package notifier

import (
	"context"
	"errors"
	"fmt"
)

// ErrPermanent 包装真实错误后表示「不要重试」。
var ErrPermanent = errors.New("permanent error")

// Permanent 把任意 err 包装成永久错误，dispatch 看到 ErrPermanent 不再重试。
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %v", ErrPermanent, err)
}

// IsPermanent 判断 err 是不是永久错误。
func IsPermanent(err error) bool {
	return errors.Is(err, ErrPermanent)
}

// Message 是渲染后的消息体；Vars 供 webhook 模板二次渲染使用。
type Message struct {
	Subject string
	Body    string
	Format  string // "text" | "markdown" | "html"
	Vars    map[string]string
}

// Notifier 是所有通道的统一发送接口；configJSON 是已解密的通道配置。
type Notifier interface {
	Type() string
	Send(ctx context.Context, configJSON []byte, msg Message) error
}

// Registry 把通道类型映射到 Notifier 实例。
//
// key 必须与 ChannelService 的类型校验和前端 ChannelType 完全一致。
var Registry = map[string]Notifier{
	"smtp":     &smtpNotifier{},
	"dingtalk": &dingtalkNotifier{},
	"wecom":    &wecomNotifier{},
	"webhook":  &webhookNotifier{},
	"log":      &logNotifier{},
}

// Get 按类型查找 Notifier，未注册返回 nil + 错误。
func Get(typ string) (Notifier, error) {
	n, ok := Registry[typ]
	if !ok {
		return nil, fmt.Errorf("未知通道类型: %s", typ)
	}
	return n, nil
}
