package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/reminder/backend/internal/logger"
	"github.com/reminder/backend/internal/middleware"
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

// FetchModels POST /api/ai/models
func (h *AIHandler) FetchModels(c *gin.Context) {
	var in services.AISettingsInput
	if err := c.ShouldBindJSON(&in); err != nil {
		abortBindErr(c, err, "请求体格式错误")
		return
	}
	models, err := h.Svc.FetchModels(c.Request.Context(), in)
	if err != nil {
		abortErr(c, err)
		return
	}
	successJSON(c, gin.H{"items": models})
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

// ChatStream POST /api/ai/chat/stream
//
// 用 SSE 推送「思考中 / 正在重试 / 完成」状态，不流式输出 token：
// 真正让人干等的是重试与工具执行。
//
// 事件格式（每条 data 都是一行 JSON）：
//
//	event: status  data: {"phase":"thinking","text":"助手正在思考…"}
//	event: retry   data: {"attempt":1,"max":10,"delay_ms":200,"reason":"服务繁忙（限流）"}
//	event: done    data: {"reply":"...","actions":[...],"retries":1}
//	event: error   data: {"code":"validation_failed","message":"..."}
//
// 一旦开始写 SSE 就不能再走 abortErr（那会追加 JSON 响应体），错误也走 event: error。
func (h *AIHandler) ChatStream(c *gin.Context) {
	var in services.ChatInput
	if err := c.ShouldBindJSON(&in); err != nil {
		// 请求体解析失败发生在写任何字节之前，仍走统一错误体
		abortBindErr(c, err, "请求体格式错误")
		return
	}

	c.Header("Content-Type", "text/event-stream; charset=utf-8")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	// 关掉反代缓冲，否则状态事件会被攒到结束才一次性吐出
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	flusher, _ := c.Writer.(http.Flusher)
	writeEvent := func(event string, data any) {
		raw, err := json.Marshal(data)
		if err != nil {
			logger.FromContext(c.Request.Context()).Warn("序列化 AI 事件失败", "event", event, "error", err)
			return
		}
		if _, err := fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", event, raw); err != nil {
			return
		}
		if flusher != nil {
			flusher.Flush()
		}
	}

	// 先推一条状态：前端读到它就能立刻把「正在思考」显示出来，
	// 而不是等第一个工具轮次开始
	writeEvent(services.ChatEventStatus, services.ChatStatus{Phase: "thinking", Text: "助手正在思考…"})

	res, err := h.Svc.ChatStream(c.Request.Context(), in, func(event string, data any) {
		writeEvent(event, data)
	})
	if err != nil {
		var ae *middleware.AppError
		if errors.As(err, &ae) {
			writeEvent(services.ChatEventError, middleware.ErrorBody(ae.Code, ae.Message, ae.Field)["error"])
			return
		}
		// SSE 已经写出，错误只能走 event: error 通知前端；
		// 服务端这边留一条 Error 日志，带上 request_id 便于和上游报错对齐。
		logger.FromContext(c.Request.Context()).Error("AI 对话流式接口未处理错误", "error", err)
		writeEvent(services.ChatEventError,
			middleware.ErrorBody(middleware.CodeInternalError, "服务器内部错误", "")["error"])
		return
	}
	writeEvent(services.ChatEventDone, res)
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
