package middleware

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"
)

// ErrorCode 是统一的应用错误码。
type ErrorCode string

const (
	CodeUnauthorized     ErrorCode = "unauthorized"
	CodeForbidden        ErrorCode = "forbidden"
	CodeNotFound         ErrorCode = "not_found"
	CodeValidationFailed ErrorCode = "validation_failed"
	CodeConflict         ErrorCode = "conflict"
	CodeRateLimited      ErrorCode = "rate_limited"
	CodeInternalError    ErrorCode = "internal_error"
)

// AppError 是 handler 可以返回 / Abort 的标准业务错误。
type AppError struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
	Field   string    `json:"field,omitempty"`
	// Status 可选地指定 HTTP 状态码；为 0 时按 Code 推断
	Status int `json:"-"`
}

func (e *AppError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("%s: %s (field=%s)", e.Code, e.Message, e.Field)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// NewAppError 构造一个 AppError。
func NewAppError(code ErrorCode, message string) *AppError {
	return &AppError{Code: code, Message: message}
}

// WithField 链式追加字段名。
func (e *AppError) WithField(field string) *AppError {
	e.Field = field
	return e
}

// WithStatus 链式覆盖 HTTP 状态码。
func (e *AppError) WithStatus(status int) *AppError {
	e.Status = status
	return e
}

// httpStatus 根据 ErrorCode 推断 HTTP 状态。
func (e *AppError) httpStatus() int {
	if e.Status != 0 {
		return e.Status
	}
	switch e.Code {
	case CodeUnauthorized:
		return http.StatusUnauthorized
	case CodeForbidden:
		return http.StatusForbidden
	case CodeNotFound:
		return http.StatusNotFound
	case CodeValidationFailed:
		return http.StatusBadRequest
	case CodeConflict:
		return http.StatusConflict
	case CodeRateLimited:
		return http.StatusTooManyRequests
	default:
		return http.StatusInternalServerError
	}
}

// ErrorBody 构造统一的错误响应体：
//
//	{"error": {"code": "not_found", "message": "...", "field": "..."}}
//
// 这是整个后端唯一的错误响应格式。所有错误出口（业务 handler、鉴权中间件、
// panic 恢复）都必须经由本函数构造响应体，避免出现「有的接口返回 error 包裹、
// 有的直接返回 code」的两套格式，前端也就只需要实现一套解析逻辑。
//
// field 为空时省略该字段。
func ErrorBody(code ErrorCode, message, field string) gin.H {
	body := gin.H{"code": code, "message": message}
	if field != "" {
		body["field"] = field
	}
	return gin.H{"error": body}
}

// ErrorHandler 是顶层中间件：
//  1. recover panic 并以 internal_error 响应；
//  2. 把 handler 通过 c.Error 注册的 AppError 统一渲染为 JSON 错误体。
//
// 同时负责把错误写进服务端日志：AppError 按状态码分级（4xx → Warn，5xx → Error），
// 未知错误一律 Error。日志里带 method / path / request_id / code / message / field，
// 让「请求日志」与「错误日志」能通过 request_id 对上同一次请求。
func ErrorHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				logWithRequest(c, slog.LevelError, "panic 恢复", "recover", r, "stack", string(debug.Stack()))
				if !c.Writer.Written() {
					c.AbortWithStatusJSON(http.StatusInternalServerError,
						ErrorBody(CodeInternalError, "服务器内部错误", ""))
				}
			}
		}()

		c.Next()

		if len(c.Errors) == 0 {
			return
		}
		// 只渲染第一个 error
		err := c.Errors[0].Err
		var ae *AppError
		if errors.As(err, &ae) {
			status := ae.httpStatus()
			level := slog.LevelWarn
			if status >= http.StatusInternalServerError {
				level = slog.LevelError
			}
			logWithRequest(c, level, "请求返回业务错误",
				"status", status,
				"error_code", string(ae.Code),
				"error_message", ae.Message,
				"field", ae.Field,
			)
			c.AbortWithStatusJSON(status, ErrorBody(ae.Code, ae.Message, ae.Field))
			return
		}
		// 未知错误，统一 500，且不泄露内部细节
		logWithRequest(c, slog.LevelError, "未处理的错误", "error", err)
		if !c.Writer.Written() {
			c.AbortWithStatusJSON(http.StatusInternalServerError,
				ErrorBody(CodeInternalError, "服务器内部错误", ""))
		}
	}
}

// logWithRequest 输出带请求上下文的日志。
//
// method / path / request_id 让一条错误日志可以脱离请求日志单独定位；
// request_id 为空（未装配 RequestID 中间件的测试路由）时自动省略。
func logWithRequest(c *gin.Context, level slog.Level, msg string, args ...any) {
	attrs := []any{
		"method", c.Request.Method,
		"path", c.Request.URL.Path,
	}
	if id := RequestIDOf(c); id != "" {
		attrs = append(attrs, "request_id", id)
	}
	slog.Log(c.Request.Context(), level, msg, append(attrs, args...)...)
}
