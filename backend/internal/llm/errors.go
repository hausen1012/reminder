package llm

import (
	"errors"

	"github.com/anthropics/anthropic-sdk-go/v2"
)

// ErrorAttrs 把厂商错误拆成结构化日志字段，不修改错误本身。
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
		// 截断保留上游原始响应体
		if raw := truncate([]byte(apiErr.RawJSON()), 1000); raw != "" {
			attrs = append(attrs, "body", raw)
		}
		return append(attrs, "error", err)
	}
	return []any{"error", err}
}

// ErrorAttrsWith 返回合并调用方字段的错误日志参数，错误属性优先覆盖同名键。
func ErrorAttrsWith(err error, attrs ...any) []any {
	out := make([]any, 0, len(attrs)+8)
	out = append(out, attrs...)
	return append(out, ErrorAttrs(err)...)
}
