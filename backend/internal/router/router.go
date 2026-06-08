package router

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/bedrock/backend/internal/config"
	"github.com/bedrock/backend/internal/crypto/secretbox"
	"github.com/bedrock/backend/internal/database"
	"github.com/bedrock/backend/internal/handlers"
	"github.com/bedrock/backend/internal/middleware"
	"github.com/bedrock/backend/internal/scheduler"
	"github.com/bedrock/backend/internal/services"
	"github.com/gin-gonic/gin"
)

// SchedulerHandles 把 scheduler 相关启动后实例返回给 main，
// 便于 main 在 shutdown 时 Stop。
type SchedulerHandles struct {
	Engine     *scheduler.Engine
	Sweeper    *scheduler.Sweeper
	ConfirmMgr *services.ConfirmRetryManager
}

// SetupResult 包装路由与调度器实例。
type SetupResult struct {
	Engine  *gin.Engine
	Handles *SchedulerHandles
}

// Setup 构建路由 + 启动调度器 + Sweeper。
//
// 与 bedrock 原版相比：返回值新增 SchedulerHandles 用于优雅停机；
// 旧调用方仍可使用 SetupEngine 取仅 *gin.Engine 的形态。
func Setup(staticFS embed.FS, cfg *config.Config) *SetupResult {
	r := gin.Default()
	r.Use(middleware.ErrorHandler())
	r.Use(middleware.Logger())
	r.Use(middleware.CORS())

	box, err := secretbox.New(cfg.SecretBoxKey)
	if err != nil {
		log.Fatalf("初始化 secretbox 失败: %v", err)
	}

	channelSvc := services.NewChannelService(database.DB, box)
	dispatchSvc := services.NewDispatchService(database.DB, channelSvc, cfg.Location)
	engine := scheduler.NewEngine(database.DB, dispatchSvc, cfg.Location)

	// 确认机制
	confirmSvc := services.NewConfirmService(database.DB, cfg)
	confirmMgr := services.NewConfirmRetryManager(database.DB, dispatchSvc, confirmSvc, cfg.Location)
	dispatchSvc.ConfirmMgr = confirmMgr
	reminderSvc := services.NewReminderService(database.DB, engine, cfg.Location, dispatchSvc, confirmMgr)

	// 启动调度器并把已 enabled 的提醒注册一遍
	engine.Start()
	if err := engine.LoadAndRegisterAll(); err != nil {
		log.Printf("调度器加载已有提醒失败: %v", err)
	}

	// 启动 sweeper
	sweeper := scheduler.NewSweeper(
		database.DB, engine, cfg.Location,
		time.Duration(cfg.SweepIntervalSec)*time.Second,
		time.Duration(cfg.MissToleranceMinutes)*time.Minute,
		30*time.Second,
	)
	sweeper.Start()

	// 自动清理日志（每天凌晨 3 点）
	logSvc := services.NewLogService(database.DB, cfg.PublicBaseURL)
	if cfg.LogAutoPurgeDays > 0 {
		purgeAfter := time.Duration(cfg.LogAutoPurgeDays) * 24 * time.Hour
		_, _ = engine.AddPurgeCron(logSvc, purgeAfter)
	}

	apiKeySvc := services.NewApiKeyService(database.DB)

	authHandler := &handlers.AuthHandler{JWTSecret: cfg.JWTSecret}
	channelHandler := &handlers.ChannelHandler{Svc: channelSvc}
	reminderHandler := &handlers.ReminderHandler{Svc: reminderSvc}
	logHandler := &handlers.LogHandler{Svc: logSvc}
	confirmHandler := &handlers.ConfirmHandler{Svc: confirmSvc}
	apiKeyHandler := &handlers.ApiKeyHandler{Svc: apiKeySvc}
	ingestHandler := &handlers.IngestHandler{ReminderSvc: reminderSvc, ApiKeySvc: apiKeySvc}

	// 确认链接（无需认证）
	r.GET("/c/:token", confirmHandler.Confirm)

	api := r.Group("/api")
	{
		healthHandler := &handlers.HealthHandler{Engine: engine, Sweeper: sweeper}
		api.GET("/health", healthHandler.Check)
		api.POST("/auth/login", authHandler.Login)
	}

	auth := api.Group("/auth")
	auth.Use(middleware.JWTAuth(cfg.JWTSecret))
	{
		auth.GET("/me", authHandler.Me)
		auth.POST("/logout", authHandler.Logout)
		auth.PUT("/password", handlers.UpdatePassword)
	}

	// Ingest API（API Key 鉴权，非 JWT）
	apiKeyVerify := func(plain string) (uint, bool) {
		k, ok := apiKeySvc.Verify(plain)
		if !ok || k == nil {
			return 0, false
		}
		return k.ID, true
	}
	ingest := api.Group("/ingest")
	ingest.Use(middleware.APIKeyAuth(apiKeyVerify, apiKeySvc.TouchLastUsed, nil))
	{
		ingest.POST("/reminders", ingestHandler.CreateReminder)
		ingest.GET("/reminders", ingestHandler.ListReminders)
		ingest.GET("/reminders/:id", ingestHandler.GetReminder)
		ingest.DELETE("/reminders/:id", ingestHandler.DeleteReminder)
		ingest.GET("/docs", ingestHandler.Docs)
	}

	protected := api.Group("")
	protected.Use(middleware.JWTAuth(cfg.JWTSecret))
	{
		channels := protected.Group("/channels")
		{
			channels.GET("", channelHandler.List)
			channels.POST("", channelHandler.Create)
			channels.GET("/:id", channelHandler.Get)
			channels.PUT("/:id", channelHandler.Update)
			channels.DELETE("/:id", channelHandler.Delete)
			channels.PATCH("/:id/toggle", channelHandler.Toggle)
			channels.POST("/:id/test", channelHandler.Test)
			channels.GET("/stats", channelHandler.Stats)
		}

		reminders := protected.Group("/reminders")
		{
			reminders.GET("", reminderHandler.List)
			reminders.POST("", reminderHandler.Create)
			reminders.POST("/preview", reminderHandler.Preview)
			reminders.GET("/upcoming", reminderHandler.Upcoming)
			reminders.GET("/:id", reminderHandler.Get)
			reminders.PUT("/:id", reminderHandler.Update)
			reminders.DELETE("/:id", reminderHandler.Delete)
			reminders.PATCH("/:id/toggle", reminderHandler.Toggle)
			reminders.POST("/:id/test", reminderHandler.Test)
		}

		logs := protected.Group("/logs")
		{
			logs.GET("", logHandler.List)
			logs.GET("/count", logHandler.PurgeCount)
			logs.GET("/:id", logHandler.GetDetail)
			logs.DELETE("", logHandler.Purge)
		}

		apikeys := protected.Group("/apikeys")
		{
			apikeys.GET("", apiKeyHandler.List)
			apikeys.POST("", apiKeyHandler.Create)
			apikeys.GET("/stats", apiKeyHandler.Stats)
			apikeys.DELETE("/:id", apiKeyHandler.Delete)
			apikeys.PATCH("/:id/toggle", apiKeyHandler.Toggle)
			apikeys.PUT("/:id/channels", apiKeyHandler.UpdateDefaultChannels)
		}
	}

	serveStaticFiles(r, staticFS)
	return &SetupResult{
		Engine: r,
		Handles: &SchedulerHandles{
			Engine:     engine,
			Sweeper:    sweeper,
			ConfirmMgr: confirmMgr,
		},
	}
}

func serveStaticFiles(r *gin.Engine, staticFS embed.FS) {
	static, err := fs.Sub(staticFS, "web")
	if err != nil {
		log.Printf("WARN: embedded web directory not found, static file serving disabled")
		return
	}
	fileServer := http.FileServer(http.FS(static))

	r.Use(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api") {
			c.Next()
			return
		}
		fileServer.ServeHTTP(c.Writer, c.Request)
		c.Abort()
	})
}
