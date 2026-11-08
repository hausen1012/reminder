package middleware

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

// BodyLimit 限制单个请求体的大小，超限返回 413 payload_too_large。
//
// 两层限制：声明了 Content-Length 超限的直接拒绝；未声明或撒谎的用 MaxBytesReader 兜住，
// 由 abortBindErr 把 *http.MaxBytesError 转成 413。
func BodyLimit(maxBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if maxBytes <= 0 {
			c.Next()
			return
		}
		if c.Request.ContentLength > maxBytes {
			c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge,
				ErrorBody(CodePayloadTooLarge, "请求体超过大小限制", ""))
			return
		}
		if c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		}
		c.Next()
	}
}

// IsBodyTooLarge 判断请求体解析错误是否来自 BodyLimit，供 abortBindErr 把 400 改成 413。
func IsBodyTooLarge(err error) bool {
	var maxErr *http.MaxBytesError
	return errors.As(err, &maxErr)
}
