package logger

import (
	"context"
	"log/slog"
)

// requestIDKey 是请求 ID 在 context 中的键。
//
// 与 middleware.ContextWithRequestID 共用同一条链路：中间件写入，
// 业务层通过 FromContext 取回，不必在每个调用点手写 request_id 字段。
type requestIDKey struct{}

// ContextWithRequestID 把请求 ID 放进 context。
//
// 只存 ID 不存 logger：handler / service 拿到的 ctx 可能被后续派生
// （例如超时控制），共享同一个 context.Context 就不会丢追踪信息。
func ContextWithRequestID(ctx context.Context, id string) context.Context {
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, requestIDKey{}, id)
}

// RequestIDFromContext 返回 context 中的请求 ID；不存在时返回空串。
func RequestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

// FromContext 返回带请求级字段的 logger。
//
// 业务层统一用它代替 slog.Info / slog.Warn：
//
//	logger.FromContext(ctx).Info("AI 对话完成", "model", model)
//
// 一次请求的多条日志因此自动带同一个 request_id，能和访问日志对上，
// 也不需要调用方记得手动加字段。没有请求上下文时返回默认 logger。
func FromContext(ctx context.Context) *slog.Logger {
	id := RequestIDFromContext(ctx)
	if id == "" {
		return slog.Default()
	}
	return slog.Default().With("request_id", id)
}
