package middleware

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

// BodyLimit 限制单个请求体的大小。
//
// 为什么需要：所有接口都会把请求体读进内存再解析 JSON，
// 没有上限时一个超大请求就能把进程内存吃满——这是不需要任何凭据就能触发的。
//
// 两层限制：
//  1. Content-Length 已声明且超限时直接拒绝，不必读任何字节；
//  2. 未声明长度（chunked）或长度撒谎时，用 http.MaxBytesReader 包住 body，
//     读超限会返回 *http.MaxBytesError，由 abortBindErr 转成 413。
//
// 超限返回 413 payload_too_large，而不是通用的 400：
// 前端能据此提示「请求过大」，运维也能把它和参数校验失败区分开。
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

// IsBodyTooLarge 判断请求体解析错误是否来自 BodyLimit。
//
// 调用方（handlers.abortBindErr）据此把 400 改成 413：
// 让用户看到的不是「请求体格式错误」这种误导性文案。
func IsBodyTooLarge(err error) bool {
	var maxErr *http.MaxBytesError
	return errors.As(err, &maxErr)
}
