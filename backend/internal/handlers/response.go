// response 集中放置所有 handler 共用的响应与参数解析辅助函数。
package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/reminder/backend/internal/middleware"
)

// parseID 从路径参数取无符号整型 ID。
func parseID(c *gin.Context, key string) (uint, error) {
	idStr := c.Param(key)
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil || id == 0 {
		return 0, middleware.NewAppError(middleware.CodeValidationFailed, "id 不合法").WithField(key)
	}
	return uint(id), nil
}

// queryInt 读取整型 query 参数；缺省或非法时返回 def。
func queryInt(c *gin.Context, key string, def int) int {
	v := c.Query(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

// queryUint 读取无符号整型 query 参数；缺省或非法时返回 0。
func queryUint(c *gin.Context, key string) uint {
	v := c.Query(key)
	if v == "" {
		return 0
	}
	n, err := strconv.ParseUint(v, 10, 64)
	if err != nil {
		return 0
	}
	return uint(n)
}

// queryBool 读取布尔 query 参数，接受 true/1；缺省或其它取值返回 nil。
func queryBool(c *gin.Context, key string) *bool {
	v := c.Query(key)
	if v == "" {
		return nil
	}
	b := v == "true" || v == "1"
	return &b
}

// abortBindErr 统一处理请求体解析失败：对外回一句可读文案，底层原因挂到 Cause 由 ErrorHandler 记录。
// 请求体超限时单独返回 413。
func abortBindErr(c *gin.Context, err error, message string) {
	if err == nil {
		return
	}
	if middleware.IsBodyTooLarge(err) {
		abortErr(c, middleware.NewAppError(middleware.CodePayloadTooLarge, "请求体超过大小限制").WithCause(err))
		return
	}
	appErr := middleware.NewAppError(middleware.CodeValidationFailed, message).WithCause(err)
	abortErr(c, appErr)
}

// abortErr 处理 service 错误并中断请求，由 ErrorHandler 统一记录日志。
func abortErr(c *gin.Context, err error) {
	_ = c.Error(err)
	c.Abort()
}

// successJSON 是面向前端的成功响应：{code, message, data} 包装。
func successJSON(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "ok",
		"data":    data,
	})
}
