// Package notifier 定义通知通道的统一接口与所有具体实现。
//
// 每个 Notifier 接收一个 Channel 配置和一段渲染后的消息（Subject + Body），
// 内部按各自协议把消息发出去。
//
// 错误约定：
//   - 返回 nil 表示成功；
//   - 返回 ErrPermanent 表示该错误不应该重试（鉴权失败、参数错误、对端业务码 != 0 等）；
//   - 返回任意其他 error 表示可重试（网络、超时、5xx 等）。
package notifier

import (
	"context"
	"errors"
	"fmt"
)

// ErrPermanent 是哨兵错误，包装真实错误后表示"不要重试"。
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

// Message 是渲染后的消息体。
//
// Subject 在某些通道（如 webhook）可能不被使用；Vars 仅在 webhook GET/POST
// 模板二次渲染时被用到，其他通道通常用不到。
type Message struct {
	Subject string
	Body    string
	Format  string // "text" | "markdown" | "html"
	Vars    map[string]string
}

// Notifier 是所有通道的统一发送接口。
//
// configJSON 是 Channel.Config 已经解密后的明文 JSON 字节流，由 ChannelService 准备。
type Notifier interface {
	Type() string
	Send(ctx context.Context, configJSON []byte, msg Message) error
}

// Registry 把通道类型字符串映射到具体的 Notifier 实例。
//
// 新增通道类型时在本文件注册即可，dispatch 与 channel_service 都从这里查询。
var Registry = map[string]Notifier{
	"smtp":     &smtpNotifier{},
	"dingtalk": &dingtalkNotifier{},
	"wecom":    &wecomNotifier{},
	"webhook":  &webhookNotifier{},
}

// Get 按类型查找 Notifier，未注册返回 nil + 错误。
func Get(typ string) (Notifier, error) {
	n, ok := Registry[typ]
	if !ok {
		return nil, fmt.Errorf("未知通道类型: %s", typ)
	}
	return n, nil
}
