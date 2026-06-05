package handlers

import (
	"net/http"
	"strconv"

	"github.com/bedrock/backend/internal/middleware"
	"github.com/bedrock/backend/internal/services"
	"github.com/gin-gonic/gin"
)

// ChannelHandler 是通道相关 HTTP 接口的入口。
type ChannelHandler struct {
	Svc *services.ChannelService
}

// 内部统一从路径取 id 的辅助。
func parseID(c *gin.Context, key string) (uint, error) {
	idStr := c.Param(key)
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil || id == 0 {
		return 0, middleware.NewAppError(middleware.CodeValidationFailed, "id 不合法").WithField(key)
	}
	return uint(id), nil
}

// abortErr 是处理 service 错误的统一入口：AppError 走中间件，其余作为 unknown error。
func abortErr(c *gin.Context, err error) {
	_ = c.Error(err)
	c.Abort()
}

// successJSON 是面向前端的成功响应：直接返回业务数据。
//
// 沿用 bedrock 的 `{code, message, data}` 包装，方便前端统一处理。
func successJSON(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "ok",
		"data":    data,
	})
}

// List GET /api/channels
func (h *ChannelHandler) List(c *gin.Context) {
	views, err := h.Svc.List()
	if err != nil {
		abortErr(c, err)
		return
	}
	successJSON(c, views)
}

// Get GET /api/channels/:id
func (h *ChannelHandler) Get(c *gin.Context) {
	id, err := parseID(c, "id")
	if err != nil {
		abortErr(c, err)
		return
	}
	v, err := h.Svc.Get(id)
	if err != nil {
		abortErr(c, err)
		return
	}
	successJSON(c, v)
}

// Create POST /api/channels
func (h *ChannelHandler) Create(c *gin.Context) {
	var in services.ChannelInput
	if err := c.ShouldBindJSON(&in); err != nil {
		abortErr(c, middleware.NewAppError(middleware.CodeValidationFailed, "请求体格式错误"))
		return
	}
	v, err := h.Svc.Create(in)
	if err != nil {
		abortErr(c, err)
		return
	}
	successJSON(c, v)
}

// Update PUT /api/channels/:id
func (h *ChannelHandler) Update(c *gin.Context) {
	id, err := parseID(c, "id")
	if err != nil {
		abortErr(c, err)
		return
	}
	var in services.ChannelInput
	if err := c.ShouldBindJSON(&in); err != nil {
		abortErr(c, middleware.NewAppError(middleware.CodeValidationFailed, "请求体格式错误"))
		return
	}
	v, err := h.Svc.Update(id, in)
	if err != nil {
		abortErr(c, err)
		return
	}
	successJSON(c, v)
}

// Delete DELETE /api/channels/:id
func (h *ChannelHandler) Delete(c *gin.Context) {
	id, err := parseID(c, "id")
	if err != nil {
		abortErr(c, err)
		return
	}
	if err := h.Svc.Delete(id); err != nil {
		abortErr(c, err)
		return
	}
	successJSON(c, nil)
}

// Toggle PATCH /api/channels/:id/toggle
func (h *ChannelHandler) Toggle(c *gin.Context) {
	id, err := parseID(c, "id")
	if err != nil {
		abortErr(c, err)
		return
	}
	v, err := h.Svc.Toggle(id)
	if err != nil {
		abortErr(c, err)
		return
	}
	successJSON(c, v)
}

// Test POST /api/channels/:id/test
// Body 可选传 {subject, body}；为空走默认占位。
func (h *ChannelHandler) Test(c *gin.Context) {
	id, err := parseID(c, "id")
	if err != nil {
		abortErr(c, err)
		return
	}
	var in struct {
		Subject string `json:"subject"`
		Body    string `json:"body"`
	}
	_ = c.ShouldBindJSON(&in)
	if err := h.Svc.Test(c.Request.Context(), id, in.Subject, in.Body); err != nil {
		// 业务错误也用 200 + data 返回，方便前端展示
		c.JSON(http.StatusOK, gin.H{
			"code":    200,
			"message": "ok",
			"data":    gin.H{"success": false, "error": err.Error()},
		})
		return
	}
	successJSON(c, gin.H{"success": true})
}
