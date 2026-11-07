package llm

import (
	"errors"

	"github.com/anthropics/anthropic-sdk-go/v2"
)

// ErrorAttrs 把厂商错误拆成结构化字段，供日志使用。
//
// 只返回 key/value 对，不修改错误本身：用户看到的仍是原本的文案，
// 而日志里能直接看到 status / request_id / 上游原文，不用再猜。
// 服务层在「对话最终失败」时也会调用它，保证上游状态码与响应体不会丢。
func ErrorAttrs(err error) []any {
	if err == nil {
		return nil
	}
	var httpErr *HTTPStatusError
	if errors.As(err, &httpErr) {
		attrs := []any{"status", httpErr.Status, "error", httpErr.Message}
		if httpErr.Body != "" {
			attrs = append(attrs, "body", httpErr.Body)
		}
		return attrs
	}
	var apiErr *anthropic.Error
	if errors.As(err, &apiErr) {
		attrs := []any{"status", apiErr.StatusCode}
		if t := string(apiErr.Type()); t != "" {
			attrs = append(attrs, "error_type", t)
		}
		if id := apiErr.RequestID; id != "" {
			attrs = append(attrs, "upstream_request_id", id)
		}
		// RawJSON 是上游原始响应体，截断后保留：中转站的真实原因常在这里
		if raw := truncate([]byte(apiErr.RawJSON()), 1000); raw != "" {
			attrs = append(attrs, "body", raw)
		}
		return append(attrs, "error", err)
	}
	return []any{"error", err}
}

// ErrorAttrsWith 返回「带调用方自定义字段」的错误日志参数。
//
// 用于把 provider / model 等本身就在日志上下文里的字段省掉：
// 传进来的 attrs 在前，错误细节在后，后者优先覆盖同名键。
func ErrorAttrsWith(err error, attrs ...any) []any {
	out := make([]any, 0, len(attrs)+8)
	out = append(out, attrs...)
	return append(out, ErrorAttrs(err)...)
}
