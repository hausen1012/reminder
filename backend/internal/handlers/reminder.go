package handlers

import (
	"strconv"
	"strings"
	"time"

	"github.com/reminder/backend/internal/middleware"
	"github.com/reminder/backend/internal/services"
	"github.com/gin-gonic/gin"
)

// ReminderHandler 是提醒 HTTP 接口。
type ReminderHandler struct {
	Svc *services.ReminderService
}

// List GET /api/reminders
func (h *ReminderHandler) List(c *gin.Context) {
	f := services.ListFilter{
		Source:    c.Query("source"),
		Search:    c.Query("search"),
		SortBy:    c.Query("sort_by"),
		SortOrder: c.Query("sort_order"),
	}
	if v := c.Query("enabled"); v != "" {
		b := v == "true" || v == "1"
		f.Enabled = &b
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
	items, total, err := h.Svc.List(f)
	if err != nil {
		abortErr(c, err)
		return
	}
	successJSON(c, gin.H{"items": items, "total": total})
}

// Get GET /api/reminders/:id
func (h *ReminderHandler) Get(c *gin.Context) {
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

// Create POST /api/reminders
func (h *ReminderHandler) Create(c *gin.Context) {
	var in services.ReminderInput
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

// Update PUT /api/reminders/:id
func (h *ReminderHandler) Update(c *gin.Context) {
	id, err := parseID(c, "id")
	if err != nil {
		abortErr(c, err)
		return
	}
	var in services.ReminderInput
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

// Delete DELETE /api/reminders/:id
func (h *ReminderHandler) Delete(c *gin.Context) {
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

// Toggle PATCH /api/reminders/:id/toggle
func (h *ReminderHandler) Toggle(c *gin.Context) {
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

// Preview POST /api/reminders/preview
// Body：{ calendar, schedule_type, schedule_spec, timezone?, count? }
func (h *ReminderHandler) Preview(c *gin.Context) {
	var in struct {
		Calendar     string         `json:"calendar"`
		ScheduleType string         `json:"schedule_type"`
		ScheduleSpec map[string]any `json:"schedule_spec"`
		Timezone     string         `json:"timezone"`
		Count        int            `json:"count"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		abortErr(c, middleware.NewAppError(middleware.CodeValidationFailed, "请求体格式错误"))
		return
	}
	in.Calendar = strings.TrimSpace(in.Calendar)
	in.ScheduleType = strings.TrimSpace(in.ScheduleType)
	times, err := h.Svc.Preview(in.Calendar, in.ScheduleType, in.ScheduleSpec, in.Timezone, in.Count)
	if err != nil {
		abortErr(c, err)
		return
	}
	successJSON(c, gin.H{"times": times})
}

// TestDryRun POST /api/reminders/test-dry
func (h *ReminderHandler) TestDryRun(c *gin.Context) {
	var in struct {
		ID            uint   `json:"id"`
		Title         string `json:"title"`
		Content       string `json:"content"`
		ContentFormat string `json:"content_format"`
		ChannelIDs    []uint `json:"channel_ids"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		abortErr(c, middleware.NewAppError(middleware.CodeValidationFailed, "请求体格式错误"))
		return
	}
	if err := h.Svc.TestDryRun(c.Request.Context(), in.Title, in.Content, in.ContentFormat, in.ChannelIDs); err != nil {
		abortErr(c, err)
		return
	}
	successJSON(c, gin.H{"success": true})
}

// Upcoming GET /api/reminders/upcoming
func (h *ReminderHandler) Upcoming(c *gin.Context) {
	scope := c.Query("scope")
	if scope == "today" {
		now := time.Now().In(h.Svc.Loc)
		year, month, day := now.Date()
		todayStart := time.Date(year, month, day, 0, 0, 0, 0, h.Svc.Loc)
		todayEnd := time.Date(year, month, day, 23, 59, 59, 999999999, h.Svc.Loc)
		items, err := h.Svc.UpcomingBetween(todayStart, todayEnd, 0)
		if err != nil {
			abortErr(c, err)
			return
		}
		successJSON(c, items)
		return
	}

	withinStr := c.DefaultQuery("within", "24h")
	limit := 10
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	within, err := time.ParseDuration(withinStr)
	if err != nil {
		within = 24 * time.Hour
	}
	items, err := h.Svc.Upcoming(within, limit)
	if err != nil {
		abortErr(c, err)
		return
	}
	successJSON(c, items)
}

// BatchDelete DELETE /api/reminders/batch
func (h *ReminderHandler) BatchDelete(c *gin.Context) {
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
