package middleware

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"
	"github.com/reminder/backend/internal/logger"
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
	CodePayloadTooLarge  ErrorCode = "payload_too_large"
	CodeInternalError    ErrorCode = "internal_error"
)

// AppError 是 handler 可以返回 / Abort 的标准业务错误。
type AppError struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
	Field   string    `json:"field,omitempty"`
	// Status 可选地指定 HTTP 状态码；为 0 时按 Code 推断
	Status int `json:"-"`
	// Cause 是底层原因（如 JSON 解析错误），只进日志不进响应体。
	//
	// 有了它，handler 就不必为了留下排查线索而自己再打一条日志：
	// ErrorHandler 统一输出一条，既带对外文案也带底层原因。
	Cause error `json:"-"`
}

func (e *AppError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("%s: %s (field=%s)", e.Code, e.Message, e.Field)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap 暴露底层原因，便于 errors.Is / errors.As 继续向下判断。
func (e *AppError) Unwrap() error { return e.Cause }

// NewAppError 构造一个 AppError。
func NewAppError(code ErrorCode, message string) *AppError {
	return &AppError{Code: code, Message: message}
}

// WithField 链式追加字段名。
func (e *AppError) WithField(field string) *AppError {
	e.Field = field
	return e
}

// WithCause 链式追加底层原因（只进日志）。
func (e *AppError) WithCause(cause error) *AppError {
	e.Cause = cause
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
	case CodePayloadTooLarge:
		return http.StatusRequestEntityTooLarge
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
			logArgs := []any{
				"status", status,
				"error_code", string(ae.Code),
				"error_message", ae.Message,
			}
			// field 为空是常态（大多数业务错误不针对具体字段），不必输出一个空 kv。
			if ae.Field != "" {
				logArgs = append(logArgs, "field", ae.Field)
			}
			// 底层原因只进日志：对外仍是同一句可读文案，排查时才看得到真因。
			if ae.Cause != nil {
				logArgs = append(logArgs, "error", ae.Cause)
			}
			logWithRequest(c, level, "请求返回业务错误", logArgs...)
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
// request_id 由 logger.FromContext 从 ctx 里取（RequestID 中间件写入），
// 因此这里只补 method / path；没有 request_id 的测试路由会退化成默认 logger。
func logWithRequest(c *gin.Context, level slog.Level, msg string, args ...any) {
	attrs := []any{
		"method", c.Request.Method,
		"path", c.Request.URL.Path,
	}
	attrs = append(attrs, args...)
	logger.FromContext(c.Request.Context()).Log(c.Request.Context(), level, msg, attrs...)
}
