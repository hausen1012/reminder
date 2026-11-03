package handlers

import (
	"github.com/gin-gonic/gin"
	"github.com/reminder/backend/internal/services"
)

// AIHandler 是 AI 助手的 HTTP 接口。
type AIHandler struct {
	Svc *services.AIService
}

// GetSettings GET /api/ai/settings
func (h *AIHandler) GetSettings(c *gin.Context) {
	out, err := h.Svc.GetSettings()
	if err != nil {
		abortErr(c, err)
		return
	}
	successJSON(c, out)
}

// UpdateSettings PUT /api/ai/settings
func (h *AIHandler) UpdateSettings(c *gin.Context) {
	var in services.AISettingsInput
	if err := c.ShouldBindJSON(&in); err != nil {
		abortBindErr(c, err, "请求体格式错误")
		return
	}
	out, err := h.Svc.SaveSettings(in)
	if err != nil {
		abortErr(c, err)
		return
	}
	successJSON(c, out)
}

// Test POST /api/ai/test
func (h *AIHandler) Test(c *gin.Context) {
	var in services.AISettingsInput
	if err := c.ShouldBindJSON(&in); err != nil {
		abortBindErr(c, err, "请求体格式错误")
		return
	}
	reply, err := h.Svc.TestConnection(c.Request.Context(), in)
	if err != nil {
		abortErr(c, err)
		return
	}
	successJSON(c, gin.H{"reply": reply})
}

// Chat POST /api/ai/chat
//
// 无状态接口：对话历史由前端随请求上传，服务端不保存聊天记录。
func (h *AIHandler) Chat(c *gin.Context) {
	var in services.ChatInput
	if err := c.ShouldBindJSON(&in); err != nil {
		abortBindErr(c, err, "请求体格式错误")
		return
	}
	res, err := h.Svc.Chat(c.Request.Context(), in)
	if err != nil {
		abortErr(c, err)
		return
	}
	successJSON(c, res)
}

// ListActions GET /api/ai/actions
func (h *AIHandler) ListActions(c *gin.Context) {
	items, err := h.Svc.ListActions(c.Query("status"))
	if err != nil {
		abortErr(c, err)
		return
	}
	successJSON(c, gin.H{"items": items})
}

// ApplyAction POST /api/ai/actions/:id/apply
func (h *AIHandler) ApplyAction(c *gin.Context) {
	act, err := h.Svc.ApplyAction(c.Param("id"))
	if err != nil {
		abortErr(c, err)
		return
	}
	successJSON(c, act)
}

// RejectAction POST /api/ai/actions/:id/reject
func (h *AIHandler) RejectAction(c *gin.Context) {
	act, err := h.Svc.RejectAction(c.Param("id"))
	if err != nil {
		abortErr(c, err)
		return
	}
	successJSON(c, act)
}
