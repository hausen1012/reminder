package handlers

import (
	"net/http"

	"github.com/bedrock/backend/internal/scheduler"
	"github.com/gin-gonic/gin"
)

// SchedulerHandler 返回调度器（Engine + Sweeper）运行时状态。
type SchedulerHandler struct {
	Engine  *scheduler.Engine
	Sweeper *scheduler.Sweeper
}

func (h *SchedulerHandler) Status(c *gin.Context) {
	entries := h.Engine.ListRegistered()
	registeredCount := len(entries)

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "ok",
		"data": gin.H{
			"engine": gin.H{
				"running":          h.Engine.IsRunning(),
				"registered_count": registeredCount,
				"entries":          entries,
			},
			"sweeper": gin.H{
				"running":          h.Sweeper.IsRunning(),
				"interval_seconds": int(h.Sweeper.Interval().Seconds()),
			},
		},
	})
}