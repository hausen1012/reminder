package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	"sync/atomic"

	"github.com/gin-gonic/gin"
)

// HeaderRequestID 是请求 ID 的响应头名。
//
// 客户端可以自带该头（例如网关生成的追踪 ID），服务端原样沿用；
// 没带或带了非法值时自动生成一个，保证每条请求日志与错误日志都能对上号。
const HeaderRequestID = "X-Request-ID"

// ctxRequestID 是请求 ID 在 gin.Context 中的键。
const ctxRequestID = "request_id"

// requestIDSeq 在 crypto/rand 不可用时提供兜底序号。
var requestIDSeq atomic.Uint64

// RequestID 为每个请求分配请求 ID，并写入响应头。
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := strings.TrimSpace(c.GetHeader(HeaderRequestID))
		// 限制长度并剔除换行：请求 ID 会进日志和响应头，过长或带换行都可能是注入。
		if id == "" || len(id) > 64 || strings.ContainsAny(id, "\r\n") {
			id = newRequestID()
		}
		c.Set(ctxRequestID, id)
		c.Header(HeaderRequestID, id)
		c.Next()
	}
}

// RequestIDOf 返回请求 ID；未装配 RequestID 中间件时返回空串。
func RequestIDOf(c *gin.Context) string {
	if c == nil {
		return ""
	}
	v, ok := c.Get(ctxRequestID)
	if !ok {
		return ""
	}
	id, _ := v.(string)
	return id
}

// newRequestID 生成 16 位十六进制请求 ID。
func newRequestID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err == nil {
		return hex.EncodeToString(b)
	}
	return "req-" + itoaUint(requestIDSeq.Add(1))
}

// itoaUint 是 strconv.FormatUint 的极简替代，避免为一个兜底分支引入 import。
func itoaUint(v uint64) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}
