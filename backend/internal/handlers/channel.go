package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/reminder/backend/internal/middleware"
	"github.com/reminder/backend/internal/services"
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
// 沿用 reminder 的 `{code, message, data}` 包装，方便前端统一处理。
func successJSON(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "ok",
		"data":    data,
	})
}

// List GET /api/channels
func (h *ChannelHandler) List(c *gin.Context) {
	limitStr := c.Query("limit")
	offsetStr := c.Query("offset")
	enabledStr := c.Query("enabled")
	search := c.Query("search")
	if limitStr == "" && offsetStr == "" && enabledStr == "" && search == "" {
		views, err := h.Svc.List()
		if err != nil {
			abortErr(c, err)
			return
		}
		successJSON(c, views)
		return
	}

	f := services.ChannelListFilter{
		Search:    search,
		SortBy:    c.Query("sort_by"),
		SortOrder: c.Query("sort_order"),
	}
	if limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil {
			f.Limit = n
		}
	}
	if offsetStr != "" {
		if n, err := strconv.Atoi(offsetStr); err == nil {
			f.Offset = n
		}
	}
	if enabledStr != "" {
		if b, err := strconv.ParseBool(enabledStr); err == nil {
			f.Enabled = &b
		}
	}
	views, total, err := h.Svc.ListPaged(f)
	if err != nil {
		abortErr(c, err)
		return
	}
	successJSON(c, gin.H{"items": views, "total": total})
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

// Stats GET /api/channels/stats?window=24h
func (h *ChannelHandler) Stats(c *gin.Context) {
	windowStr := c.DefaultQuery("window", "24h")
	window, err := time.ParseDuration(windowStr)
	if err != nil {
		window = 24 * time.Hour
	}
	stats, err := h.Svc.Stats(window)
	if err != nil {
		abortErr(c, err)
		return
	}
	successJSON(c, stats)
}

// BatchDelete DELETE /api/channels/batch
func (h *ChannelHandler) BatchDelete(c *gin.Context) {
	var in struct {
		IDs []uint `json:"ids"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		abortErr(c, middleware.NewAppError(middleware.CodeValidationFailed, "请求体格式错误"))
		return
	}
	if len(in.IDs) == 0 {
		abortErr(c, middleware.NewAppError(middleware.CodeValidationFailed, "ids 不能为空"))
		return
	}
	if err := h.Svc.BatchDelete(in.IDs); err != nil {
		abortErr(c, err)
		return
	}
	successJSON(c, nil)
}

// TestDryRun POST /api/channels/test-dry
func (h *ChannelHandler) TestDryRun(c *gin.Context) {
	var in struct {
		ID     uint           `json:"id"`
		Type   string         `json:"type"`
		Config map[string]any `json:"config"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		abortErr(c, middleware.NewAppError(middleware.CodeValidationFailed, "请求体格式错误"))
		return
	}
	if err := h.Svc.DryRun(c.Request.Context(), in.Type, in.Config); err != nil {
		abortErr(c, err)
		return
	}
	successJSON(c, gin.H{"success": true})
}
