package router

import (
	"embed"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/reminder/backend/internal/config"
	"github.com/reminder/backend/internal/crypto/secretbox"
	"github.com/reminder/backend/internal/database"
	"github.com/reminder/backend/internal/handlers"
	"github.com/reminder/backend/internal/middleware"
	"github.com/reminder/backend/internal/scheduler"
	"github.com/reminder/backend/internal/services"
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
// 与 reminder 原版相比：返回值新增 SchedulerHandles 用于优雅停机；
// 旧调用方仍可使用 SetupEngine 取仅 *gin.Engine 的形态。
func Setup(staticFS embed.FS, cfg *config.Config) *SetupResult {
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.SlogLogger())
	r.Use(middleware.ErrorHandler())
	r.Use(middleware.CORS())

	box, err := secretbox.New(cfg.EncryptionKey)
	if err != nil {
		slog.Error("初始化 secretbox 失败", "error", err)
			panic(err)
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
		slog.Error("调度器加载已有提醒失败", "error", err)
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
	logSvc := services.NewLogService(database.DB, cfg.BaseURL)
	if cfg.LogAutoPurgeDays > 0 {
		purgeAfter := time.Duration(cfg.LogAutoPurgeDays) * 24 * time.Hour
		_, _ = engine.AddPurgeCron(logSvc, purgeAfter)
	}

	tokenSvc := services.NewTokenService(database.DB)
	configSvc := services.NewConfigService(database.DB)

	authHandler := &handlers.AuthHandler{JWTSecret: cfg.JWTSecret}
	channelHandler := &handlers.ChannelHandler{Svc: channelSvc}
	reminderHandler := &handlers.ReminderHandler{Svc: reminderSvc}
	logHandler := &handlers.LogHandler{Svc: logSvc}
	confirmHandler := &handlers.ConfirmHandler{Svc: confirmSvc}
	tokenHandler := &handlers.TokenHandler{Svc: tokenSvc}
	externalHandler := &handlers.ExternalHandler{ReminderSvc: reminderSvc, TokenSvc: tokenSvc, ChannelSvc: channelSvc}
	schedulerHandler := &handlers.SchedulerHandler{Engine: engine, Sweeper: sweeper}
		configHandler := &handlers.ConfigHandler{Svc: configSvc}

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

	// 外部 API v1（令牌鉴权，非 JWT）
	tokenVerify := func(plain string) (uint, bool) {
		k, ok := tokenSvc.Verify(plain)
		if !ok || k == nil {
			return 0, false
		}
		return k.ID, true
	}
	externalV1 := api.Group("/external/v1")
	externalV1.Use(middleware.TokenAuth(tokenVerify, tokenSvc.TouchLastUsed, nil))
	{
		externalV1.POST("/reminders", externalHandler.CreateReminder)
		externalV1.GET("/reminders", externalHandler.ListReminders)
		externalV1.GET("/reminders/:id", externalHandler.GetReminder)
		externalV1.PUT("/reminders/:id", externalHandler.UpdateReminder)
		externalV1.DELETE("/reminders/:id", externalHandler.DeleteReminder)
		externalV1.GET("/channels", externalHandler.ListChannels)
	}

	// 文档页对外开放，无需鉴权
	api.GET("/external/v1/docs", externalHandler.Docs)

	protected := api.Group("")
	protected.Use(middleware.JWTAuth(cfg.JWTSecret))
	{
		channels := protected.Group("/channels")
		{
			channels.GET("", channelHandler.List)
			channels.POST("", channelHandler.Create)
			channels.GET("/:id", channelHandler.Get)
			channels.PUT("/:id", channelHandler.Update)
			channels.DELETE("/batch", channelHandler.BatchDelete)  // 新增（放在 /:id 之前）
			channels.DELETE("/:id", channelHandler.Delete)
			channels.PATCH("/:id/toggle", channelHandler.Toggle)
			channels.POST("/test-dry", channelHandler.TestDryRun)
			channels.GET("/stats", channelHandler.Stats)
		}

		reminders := protected.Group("/reminders")
		{
			reminders.GET("", reminderHandler.List)
			reminders.POST("", reminderHandler.Create)
			reminders.POST("/preview", reminderHandler.Preview)
			reminders.GET("/upcoming", reminderHandler.Upcoming)
			reminders.DELETE("/batch", reminderHandler.BatchDelete)
			reminders.GET("/:id", reminderHandler.Get)
			reminders.PUT("/:id", reminderHandler.Update)
			reminders.DELETE("/:id", reminderHandler.Delete)
			reminders.PATCH("/:id/toggle", reminderHandler.Toggle)
			reminders.POST("/test-dry", reminderHandler.TestDryRun)
		}

		logs := protected.Group("/logs")
		{
			logs.GET("", logHandler.List)
			logs.GET("/count", logHandler.PurgeCount)
			logs.GET("/:id", logHandler.GetDetail)
			logs.DELETE("/batch", logHandler.BatchDelete)
			logs.DELETE("", logHandler.Purge)
		}

		tokens := protected.Group("/tokens")
		{
			tokens.GET("", tokenHandler.List)
			tokens.POST("", tokenHandler.Create)
			tokens.GET("/stats", tokenHandler.Stats)
			tokens.GET("/:id", tokenHandler.Get)
			tokens.GET("/:id/plaintext", tokenHandler.GetPlaintext)
			tokens.DELETE("/:id", tokenHandler.Delete)
			tokens.PATCH("/:id/toggle", tokenHandler.Toggle)
			tokens.PUT("/:id/channels", tokenHandler.UpdateDefaultChannels)
		}

		scheduler := protected.Group("/scheduler")
		scheduler.GET("/status", schedulerHandler.Status)

		cfgRoute := protected.Group("/config")
		cfgRoute.GET("", configHandler.GetAll)
		cfgRoute.PUT("", configHandler.Update)
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
		slog.Warn("嵌入式 web 目录不存在，静态文件服务已禁用")
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
