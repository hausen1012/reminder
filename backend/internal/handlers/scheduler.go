package handlers

import (
	"net/http"
	"strconv"

	"github.com/reminder/backend/internal/scheduler"
	"github.com/gin-gonic/gin"
)

// SchedulerHandler 返回调度器（Engine + Sweeper）运行时状态。
type SchedulerHandler struct {
	Engine  *scheduler.Engine
	Sweeper *scheduler.Sweeper
}

func (h *SchedulerHandler) Status(c *gin.Context) {
	// 分页参数
	offset, limit := 0, 0
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	if v := c.Query("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			offset = n
		}
	}

	var entries []scheduler.RegisteredEntry
	registeredCount := 0
	var entriesTotal int

	if limit > 0 {
		entries, entriesTotal = h.Engine.ListRegisteredSorted(offset, limit)
		registeredCount = h.Engine.RegisteredCount()
	} else {
		entries = h.Engine.ListRegistered()
		registeredCount = len(entries)
		entriesTotal = registeredCount
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "ok",
		"data": gin.H{
			"engine": gin.H{
				"running":          h.Engine.IsRunning(),
				"registered_count": registeredCount,
				"entries":          entries,
				"entries_total":    entriesTotal,
			},
			"sweeper": gin.H{
				"running":          h.Sweeper.IsRunning(),
				"interval_seconds": int(h.Sweeper.Interval().Seconds()),
			},
		},
	})
}