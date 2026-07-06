package handlers

import (
	"strconv"
	"time"

	"github.com/reminder/backend/internal/middleware"
	"github.com/reminder/backend/internal/services"
	"github.com/gin-gonic/gin"
)

// LogHandler 是日志相关接口。
type LogHandler struct {
	Svc *services.LogService
}

// List GET /api/logs
func (h *LogHandler) List(c *gin.Context) {
	f := services.LogFilter{
		Status: c.Query("status"),
		Source: c.Query("source"),
		Search: c.Query("search"),
	}
	if v := c.Query("reminder_id"); v != "" {
		if n, err := strconv.ParseUint(v, 10, 64); err == nil {
			f.ReminderID = uint(n)
		}
	}
	if v := c.Query("since"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			f.Since = &t
		}
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

// GetDetail GET /api/logs/:id
func (h *LogHandler) GetDetail(c *gin.Context) {
	id, err := parseID(c, "id")
	if err != nil {
		abortErr(c, err)
		return
	}
	v, err := h.Svc.GetDetail(id)
	if err != nil {
		abortErr(c, err)
		return
	}
	successJSON(c, v)
}

// Purge DELETE /api/logs
// Query params: older_than=7d|30d, all=true
//
// 先调 GET /api/logs/count?older_than=... 查看数量做二次确认。
func (h *LogHandler) Purge(c *gin.Context) {
	olderThan, all := parsePurgeParams(c)
	count, err := h.Svc.Purge(olderThan, all)
	if err != nil {
		abortErr(c, err)
		return
	}
	successJSON(c, gin.H{"deleted": count})
}

// PurgeCount GET /api/logs/count
func (h *LogHandler) PurgeCount(c *gin.Context) {
	olderThan, all := parsePurgeParams(c)
	count, err := h.Svc.PurgeCount(olderThan, all)
	if err != nil {
		abortErr(c, err)
		return
	}
	successJSON(c, gin.H{"count": count})
}

func parsePurgeParams(c *gin.Context) (time.Duration, bool) {
	all := c.Query("all") == "true"
	var olderThan time.Duration
	if v := c.Query("older_than"); v != "" {
		var err error
		olderThan, err = time.ParseDuration(v)
		if err != nil {
			olderThan = 0
		}
	}
	return olderThan, all
}

// BatchDelete DELETE /api/logs/batch
func (h *LogHandler) BatchDelete(c *gin.Context) {
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
