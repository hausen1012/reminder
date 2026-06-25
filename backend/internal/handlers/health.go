package handlers

import (
	"net/http"
	"time"

	"github.com/reminder/backend/internal/database"
	"github.com/reminder/backend/internal/scheduler"
	"github.com/gin-gonic/gin"
)

// HealthHandler 返回带调度器状态的健康检查。
type HealthHandler struct {
	Engine  *scheduler.Engine
	Sweeper *scheduler.Sweeper
}

func (h *HealthHandler) Check(c *gin.Context) {
	dbAlive := false
	sqlDB, err := database.DB.DB()
	if err == nil {
		dbAlive = sqlDB.Ping() == nil
	}

	var reminderCount, logCount int64
	database.DB.Raw("SELECT COUNT(*) FROM reminders").Scan(&reminderCount)
	database.DB.Raw("SELECT COUNT(*) FROM delivery_logs").Scan(&logCount)

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "ok",
		"data": gin.H{
			"status":             "healthy",
			"db_alive":           dbAlive,
			"db_reminders":       reminderCount,
			"db_delivery_logs":   logCount,
			"scheduler": gin.H{
				"registered":   h.Engine.RegisteredCount(),
				"last_fire_at": h.Engine.LastFireTime().Format(time.RFC3339),
			},
			"sweeper": gin.H{
				"running": h.Sweeper.IsRunning(),
			},
		},
	})
}
