package handlers

import (
	"log/slog"
	"net/http"
	"strconv"
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
//
// 状态码即结论：数据库可连且调度器运行时返回 200 + healthy，否则 503 + unhealthy。
func (h *HealthHandler) Check(c *gin.Context) {
	dbAlive := false
	if h.DB != nil {
		if sqlDB, err := h.DB.DB(); err == nil {
			dbAlive = sqlDB.Ping() == nil
		}
	}

	engineRunning := h.Engine != nil && h.Engine.IsRunning()
	sweeperRunning := h.Sweeper != nil && h.Sweeper.IsRunning()

	var reminderCount, logCount int64
	if h.DB != nil && dbAlive {
		if err := h.DB.Model(&models.Reminder{}).Count(&reminderCount).Error; err != nil {
			slog.Warn("健康检查统计提醒数失败", "error", err)
		}
		if err := h.DB.Model(&models.DeliveryLog{}).Count(&logCount).Error; err != nil {
			slog.Warn("健康检查统计日志数失败", "error", err)
		}
	}

	status := "healthy"
	if !dbAlive || !engineRunning {
		status = "unhealthy"
	}

	var lastFireAt string
	registered := 0
	if h.Engine != nil {
		registered = h.Engine.RegisteredCount()
		if t := h.Engine.LastFireTime(); !t.IsZero() {
			lastFireAt = t.Format(time.RFC3339)
		}
	}

	body := gin.H{
		"code":    200,
		"message": "ok",
		"data": gin.H{
			"status":           status,
			"db_alive":         dbAlive,
			"db_reminders":     reminderCount,
			"db_delivery_logs": logCount,
			"scheduler": gin.H{
				"running":      engineRunning,
				"registered":   registered,
				"last_fire_at": lastFireAt,
			},
			"sweeper": gin.H{
				"running": sweeperRunning,
			},
		},
	}

	httpStatus := http.StatusOK
	if status != "healthy" {
		httpStatus = http.StatusServiceUnavailable
		body["code"] = httpStatus
		body["message"] = "服务不可用"
	}
	c.JSON(httpStatus, body)
}

// SchedulerHandler 返回调度器（Engine + Sweeper）运行时状态。
//
// 与 HealthHandler 放在一起：两者都只读调度器运行态，且共用同一组依赖。
type SchedulerHandler struct {
	Engine  *scheduler.Engine
	Sweeper *scheduler.Sweeper
}

// Status GET /api/scheduler/status（支持 limit/offset 分页）
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

	successJSON(c, gin.H{
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
	})
}
