// response 集中放置所有 handler 共用的响应与参数解析辅助函数。
//
// 这些函数原先散落在 channel.go 顶部，导致「通道相关的文件」承担了全局职责，
// 现独立成文件，任何 handler 都可以直接复用。
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

// abortErr 是处理 service 错误的统一入口：AppError 走中间件渲染，其余视为 unknown。
func abortErr(c *gin.Context, err error) {
	_ = c.Error(err)
	c.Abort()
}

// successJSON 是面向前端的成功响应：`{code, message, data}` 包装。
func successJSON(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "ok",
		"data":    data,
	})
}
