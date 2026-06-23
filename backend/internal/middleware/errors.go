package middleware

import (
	"errors"
	"fmt"
	"log"
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
	CodeConflict        ErrorCode = "conflict"
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

// ErrorHandler 是顶层中间件：
//  1. recover panic 并以 internal_error 响应；
//  2. 把 handler 通过 c.Error 注册的 AppError 统一渲染为 JSON 错误体。
func ErrorHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[ERROR] panic: %v\n%s", r, debug.Stack())
				if !c.Writer.Written() {
					c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
						"error": gin.H{
							"code":    CodeInternalError,
							"message": "服务器内部错误",
						},
					})
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
			body := gin.H{"code": ae.Code, "message": ae.Message}
			if ae.Field != "" {
				body["field"] = ae.Field
			}
			c.AbortWithStatusJSON(ae.httpStatus(), gin.H{"error": body})
			return
		}
		// 未知错误，统一 500
		log.Printf("[ERROR] unhandled error: %v", err)
		if !c.Writer.Written() {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error": gin.H{
					"code":    CodeInternalError,
					"message": "服务器内部错误",
				},
			})
		}
	}
}
