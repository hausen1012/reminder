package router

import (
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/reminder/backend/internal/config"
	"github.com/reminder/backend/internal/crypto/secretbox"
	"github.com/reminder/backend/internal/database"
	"github.com/reminder/backend/internal/handlers"
	"github.com/reminder/backend/internal/middleware"
	"github.com/reminder/backend/internal/scheduler"
	"github.com/reminder/backend/internal/services"
	"github.com/reminder/backend/internal/timezone"
)

// SchedulerHandles 把 scheduler 相关启动后实例返回给 main，
// 便于 main 在 shutdown 时 Stop。
type SchedulerHandles struct {
	Engine     *scheduler.Engine
	Sweeper    *scheduler.Sweeper
	ConfirmMgr *services.ConfirmRetryManager
	TZ         *timezone.Provider
}

// SetupResult 包装路由与调度器实例。
type SetupResult struct {
	Engine  *gin.Engine
	Handles *SchedulerHandles
}

// Setup 构建路由 + 启动调度器 + Sweeper。
//
// 返回值包含 gin.Engine 与 SchedulerHandles（供 main 在优雅停机时 Stop 各后台组件）。
// 初始化失败时返回 error，由调用方决定如何退出，避免在库内 panic。
func Setup(staticFS embed.FS, cfg *config.Config) (*SetupResult, error) {
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.SlogLogger())
	r.Use(middleware.ErrorHandler())
	r.Use(middleware.CORS())

	box, err := secretbox.New(cfg.EncryptionKey)
	if err != nil {
		return nil, fmt.Errorf("初始化 secretbox 失败: %w", err)
	}

	// 站点时区：先用环境变量 TIMEZONE 给出的默认值，随后由库里的设置覆盖。
	// 这一层是可变引用，设置页改时区后所有服务立刻读到新值。
	tz := timezone.New(cfg.Timezone)

	channelSvc := services.NewChannelService(database.DB, box, tz)
	dispatchSvc := services.NewDispatchService(database.DB, channelSvc, tz)
	engine := scheduler.NewEngine(database.DB, dispatchSvc, tz)

	// 确认机制
	confirmSvc := services.NewConfirmService(database.DB, cfg)
	confirmMgr := services.NewConfirmRetryManager(database.DB, dispatchSvc, confirmSvc, tz)
	dispatchSvc.ConfirmMgr = confirmMgr
	reminderSvc := services.NewReminderService(database.DB, engine, tz, dispatchSvc, confirmMgr)

	configSvc := services.NewConfigService(database.DB, tz)
	configSvc.OnTimezoneChanged = engine.ReloadTimezone
	// 读库里的站点时区（若存在）并应用，必须早于调度器注册
	configSvc.LoadTimezone()

	// 同步进程默认时区，供日志按天切分、确认页时间展示等零星用途。
	// 只在启动阶段赋一次值：time.Local 是全局变量，运行期写入会是数据竞争。
	time.Local = tz.Get()

	// 启动调度器并把已 enabled 的提醒注册一遍
	engine.Start()
	if err := engine.LoadAndRegisterAll(); err != nil {
		slog.Error("调度器加载已有提醒失败", "error", err)
	}

	// 启动 sweeper
	sweeper := scheduler.NewSweeper(
		database.DB, engine, tz,
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
	userSvc := services.NewUserService(database.DB)

	authHandler := &handlers.AuthHandler{JWTSecret: cfg.JWTSecret, UserSvc: userSvc}
	userHandler := &handlers.UserHandler{Svc: userSvc}
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
		healthHandler := &handlers.HealthHandler{DB: database.DB, Engine: engine, Sweeper: sweeper}
		api.GET("/health", healthHandler.Check)
		api.POST("/auth/login", authHandler.Login)
	}

	auth := api.Group("/auth")
	auth.Use(middleware.JWTAuth(cfg.JWTSecret))
	{
		auth.GET("/me", authHandler.Me)
		auth.POST("/logout", authHandler.Logout)
		auth.PUT("/password", userHandler.UpdatePassword)
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
			channels.DELETE("/batch", channelHandler.BatchDelete) // 新增（放在 /:id 之前）
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
			TZ:         tz,
		},
	}, nil
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
		// 确认链接路径直接交由路由处理，不返回 SPA
		if strings.HasPrefix(c.Request.URL.Path, "/c/") {
			c.Next()
			return
		}
		fileServer.ServeHTTP(c.Writer, c.Request)
		c.Abort()
	})
}
