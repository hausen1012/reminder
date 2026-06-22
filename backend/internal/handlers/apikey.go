// apikey 处理面板上 API Key 的 CRUD。
package handlers

import (
	"strconv"

	"github.com/bedrock/backend/internal/middleware"
	"github.com/bedrock/backend/internal/services"
	"github.com/gin-gonic/gin"
)

// ApiKeyHandler 是面板上 API Key 管理接口。
type ApiKeyHandler struct {
	Svc *services.ApiKeyService
}

// List GET /api/apikeys
func (h *ApiKeyHandler) List(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	search := c.Query("search")
	if limit < 1 || limit > 100 {
		limit = 10
	}
	if offset < 0 {
		offset = 0
	}

	views, total, err := h.Svc.ListViews(limit, offset, search)
	if err != nil {
		abortErr(c, err)
		return
	}
	successJSON(c, gin.H{
		"items": views,
		"total": total,
	})
}

// Get GET /api/apikeys/:id
func (h *ApiKeyHandler) Get(c *gin.Context) {
	id, err := parseID(c, "id")
	if err != nil {
		abortErr(c, err)
		return
	}
	view, err := h.Svc.GetView(id)
	if err != nil {
		abortErr(c, err)
		return
	}
	successJSON(c, view)
}

// GetPlaintext GET /api/apikeys/:id/plaintext
func (h *ApiKeyHandler) GetPlaintext(c *gin.Context) {
	id, err := parseID(c, "id")
	if err != nil {
		abortErr(c, err)
		return
	}
	plaintext, err := h.Svc.GetPlaintext(id)
	if err != nil {
		abortErr(c, err)
		return
	}
	successJSON(c, gin.H{"plaintext": plaintext})
}

// Create POST /api/apikeys
func (h *ApiKeyHandler) Create(c *gin.Context) {
	var in struct {
		Name              string `json:"name"`
		DefaultChannelIDs []uint `json:"default_channel_ids"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		abortErr(c, middleware.NewAppError(middleware.CodeValidationFailed, "请求体格式错误"))
		return
	}
	plain, key, err := h.Svc.Create(in.Name, in.DefaultChannelIDs)
	if err != nil {
		abortErr(c, err)
		return
	}
	successJSON(c, gin.H{
		"plaintext": plain,
		"key":       key,
	})
}

// Toggle PATCH /api/apikeys/:id/toggle
func (h *ApiKeyHandler) Toggle(c *gin.Context) {
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

// Delete DELETE /api/apikeys/:id
func (h *ApiKeyHandler) Delete(c *gin.Context) {
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

// UpdateDefaultChannels PUT /api/apikeys/:id/channels
func (h *ApiKeyHandler) UpdateDefaultChannels(c *gin.Context) {
	id, err := parseID(c, "id")
	if err != nil {
		abortErr(c, err)
		return
	}
	var in struct {
		ChannelIDs []uint `json:"channel_ids"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		abortErr(c, middleware.NewAppError(middleware.CodeValidationFailed, "请求体格式错误"))
		return
	}
	if err := h.Svc.UpdateDefaultChannels(id, in.ChannelIDs); err != nil {
		abortErr(c, err)
		return
	}
	successJSON(c, nil)
}

// Stats GET /api/apikeys/stats
func (h *ApiKeyHandler) Stats(c *gin.Context) {
	views, _, err := h.Svc.ListViews(1000, 0, "")
	if err != nil {
		abortErr(c, err)
		return
	}
	type statItem struct {
		ID      uint   `json:"id"`
		Name    string `json:"name"`
		Usage24 int64  `json:"usage_24h"`
	}
	stats := make([]statItem, 0, len(views))
	for _, v := range views {
		stats = append(stats, statItem{ID: v.ID, Name: v.Name, Usage24: v.Usage24h})
	}
	successJSON(c, stats)
}
