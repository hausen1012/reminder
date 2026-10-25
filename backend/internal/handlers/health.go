package handlers

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/reminder/backend/internal/models"
	"github.com/reminder/backend/internal/scheduler"
	"gorm.io/gorm"
)

// HealthHandler 返回带调度器状态的健康检查。
type HealthHandler struct {
	DB      *gorm.DB
	Engine  *scheduler.Engine
	Sweeper *scheduler.Sweeper
}

// Check GET /api/health
func (h *HealthHandler) Check(c *gin.Context) {
	dbAlive := false
	if h.DB != nil {
		if sqlDB, err := h.DB.DB(); err == nil {
			dbAlive = sqlDB.Ping() == nil
		}
	}

	var reminderCount, logCount int64
	if h.DB != nil {
		h.DB.Model(&models.Reminder{}).Count(&reminderCount)
		h.DB.Model(&models.DeliveryLog{}).Count(&logCount)
	}

	successJSON(c, gin.H{
		"status":           "healthy",
		"db_alive":         dbAlive,
		"db_reminders":     reminderCount,
		"db_delivery_logs": logCount,
		"scheduler": gin.H{
			"registered":   h.Engine.RegisteredCount(),
			"last_fire_at": h.Engine.LastFireTime().Format(time.RFC3339),
		},
		"sweeper": gin.H{
			"running": h.Sweeper.IsRunning(),
		},
	})
}
