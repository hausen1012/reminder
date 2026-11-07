// response 集中放置所有 handler 共用的响应与参数解析辅助函数。
//
// 独立成文件，任何 handler 都可以直接复用，避免某个业务文件承担全局职责。
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

// abortBindErr 统一处理请求体解析失败。
//
// 对外只回一句用户能看懂的话（不暴露 JSON 字段名等内部细节），
// 但把底层原因（缺字段、类型不对、JSON 语法错）挂到 AppError.Cause 上：
// ErrorHandler 会把它写进「请求返回业务错误」那条日志，不必在这里再打一条，
// 否则同一次失败在日志里出现两遍。
//
// 请求体超过 BodyLimit 上限时单独返回 413：这种情况既不是用户填错，
// 也不该让人从「格式错误」里猜。
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

// abortErr 是处理 service 错误的统一入口：AppError 走中间件渲染，其余视为 unknown。
//
// 这里不写日志：ErrorHandler 在渲染响应时统一记录（带 request_id 与分级级别），
// 若此处也记一遍，同一次失败会在日志里出现两条，反而干扰排查。
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
