package logger

import (
	"context"
	"log/slog"
)

// requestIDKey 是请求 ID 在 context 中的键，与 middleware.ContextWithRequestID 共用。
type requestIDKey struct{}

// ContextWithRequestID 把请求 ID 放进 context；只存 ID 不存 logger，派生 ctx 不会丢追踪信息。
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

// FromContext 返回带 request_id 的 logger，业务层统一用它代替 slog.Info / slog.Warn；
// 没有请求上下文时返回默认 logger。
func FromContext(ctx context.Context) *slog.Logger {
	id := RequestIDFromContext(ctx)
	if id == "" {
		return slog.Default()
	}
	return slog.Default().With("request_id", id)
}
