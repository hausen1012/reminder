// external 处理外部 API v1 调用。
//
// 与面板使用的 ReminderHandler 共享 ReminderService，但鉴权使用令牌而非 JWT。
// 提醒创建时强制 source=api，未指定通道时回退到 Key 的默认通道。
//
// 本文件只放接口实现：外部 API 的文档唯一来源是 docs-site/api/，
// 改接口时同步改文档，不在代码里再维护一份内嵌 HTML。
package handlers

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/reminder/backend/internal/middleware"
	"github.com/reminder/backend/internal/services"
)

// ExternalHandler 是外部 API v1 的处理器。
type ExternalHandler struct {
	ReminderSvc *services.ReminderService
	TokenSvc    *services.TokenService
	ChannelSvc  *services.ChannelService
}

// CreateReminder POST /api/external/v1/reminders
func (h *ExternalHandler) CreateReminder(c *gin.Context) {
	var in services.ReminderInput
	if err := c.ShouldBindJSON(&in); err != nil {
		abortBindErr(c, err, "请求体格式错误")
		return
	}

	// 强制 source=api
	in.Source = "api"

	// 未指定通道时回退到令牌默认通道
	tokenID, _ := c.Get("token_id")
	if keyID, ok := tokenID.(uint); ok {
		in.TokenID = &keyID
		if len(in.ChannelIDs) == 0 {
			in.ChannelIDs = h.TokenSvc.DefaultChannelIDs(keyID)
		}
	}

	v, err := h.ReminderSvc.Create(in)
	if err != nil {
		abortErr(c, err)
		return
	}
	successJSON(c, v)
}

// GetReminder GET /api/external/v1/reminders/:id
func (h *ExternalHandler) GetReminder(c *gin.Context) {
	id, err := parseID(c, "id")
	if err != nil {
		abortErr(c, err)
		return
	}
	v, err := h.ReminderSvc.Get(id)
	if err != nil {
		abortErr(c, err)
		return
	}
	// 限制只能查看本 Key 创建的
	if !h.belongsToKey(v.TokenID, c) {
		abortErr(c, middleware.NewAppError(middleware.CodeNotFound, "提醒不存在"))
		return
	}
	successJSON(c, v)
}

// ListReminders GET /api/external/v1/reminders
func (h *ExternalHandler) ListReminders(c *gin.Context) {
	tokenID, _ := c.Get("token_id")
	f := services.ListFilter{
		Search: c.Query("search"),
	}
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			f.Limit = n
		}
	}
	if v := c.Query("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			f.Offset = n
		}
	}
	if keyID, ok := tokenID.(uint); ok {
		f.TokenID = &keyID
	}

	items, total, err := h.ReminderSvc.List(f)
	if err != nil {
		abortErr(c, err)
		return
	}
	successJSON(c, gin.H{"items": items, "total": total})
}

// DeleteReminder DELETE /api/external/v1/reminders/:id
func (h *ExternalHandler) DeleteReminder(c *gin.Context) {
	id, err := parseID(c, "id")
	if err != nil {
		abortErr(c, err)
		return
	}
	// 先查，确认是本 Key 创建的
	v, err := h.ReminderSvc.Get(id)
	if err != nil {
		abortErr(c, err)
		return
	}
	if !h.belongsToKey(v.TokenID, c) {
		abortErr(c, middleware.NewAppError(middleware.CodeNotFound, "提醒不存在"))
		return
	}
	if err := h.ReminderSvc.Delete(id); err != nil {
		abortErr(c, err)
		return
	}
	successJSON(c, nil)
}

// UpdateReminder PUT /api/external/v1/reminders/:id
func (h *ExternalHandler) UpdateReminder(c *gin.Context) {
	id, err := parseID(c, "id")
	if err != nil {
		abortErr(c, err)
		return
	}

	// 确认是本 Key 创建的
	v, err := h.ReminderSvc.Get(id)
	if err != nil {
		abortErr(c, err)
		return
	}
	if !h.belongsToKey(v.TokenID, c) {
		abortErr(c, middleware.NewAppError(middleware.CodeNotFound, "提醒不存在"))
		return
	}

	var in services.ReminderInput
	if err := c.ShouldBindJSON(&in); err != nil {
		abortBindErr(c, err, "请求体格式错误")
		return
	}

	// 强制 source=api，保留原 token
	in.Source = "api"
	tokenID, _ := c.Get("token_id")
	if keyID, ok := tokenID.(uint); ok {
		in.TokenID = &keyID
	}

	updated, err := h.ReminderSvc.Update(id, in)
	if err != nil {
		abortErr(c, err)
		return
	}
	successJSON(c, updated)
}

// ListChannels GET /api/external/v1/channels
//
// 返回所有通知渠道列表，供外部调用方选择 channel_id。
func (h *ExternalHandler) ListChannels(c *gin.Context) {
	views, err := h.ChannelSvc.List()
	if err != nil {
		abortErr(c, err)
		return
	}
	successJSON(c, views)
}

func (h *ExternalHandler) belongsToKey(reminderKeyID *uint, c *gin.Context) bool {
	tokenID, exists := c.Get("token_id")
	if !exists {
		return false
	}
	keyID, ok := tokenID.(uint)
	if !ok {
		return false
	}
	if reminderKeyID == nil {
		return false
	}
	return *reminderKeyID == keyID
}
