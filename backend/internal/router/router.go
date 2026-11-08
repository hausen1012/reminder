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

// applyMiddleware 装配全局中间件。
//
// 顺序是契约：
//
//	Recovery      → 兜住下游 panic，必须第一个
//	RequestID     → SlogLogger / ErrorHandler 都要读它
//	CORS          → 必须在 BodyLimit 之前：CORS 之后提前 Abort 的响应会缺
//	                 Access-Control-* 头，浏览器把 413 当成不可读的跨域失败
//	SlogLogger    → 记录访问日志，包括被拦截的请求
//	BodyLimit     → 读 body 前拦住超大请求
//	ErrorHandler  → 渲染 c.Error 里注册的 AppError
//
// 拆成独立函数是为了让顺序能被单测断言（见 router/middleware_order_test.go）。
func applyMiddleware(r *gin.Engine, cfg *config.Config) {
	r.Use(gin.Recovery())
	r.Use(middleware.RequestID())
	r.Use(middleware.CORS())
	r.Use(middleware.SlogLogger())
	r.Use(middleware.BodyLimit(cfg.MaxBodyMB << 20))
	r.Use(middleware.ErrorHandler())
}

// Setup 构建路由 + 启动调度器 + Sweeper。
//
// 返回值包含 gin.Engine 与 SchedulerHandles（供 main 在优雅停机时 Stop 各后台组件）。
// 初始化失败时返回 error，由调用方决定如何退出，避免在库内 panic。
func Setup(staticFS embed.FS, cfg *config.Config) (*SetupResult, error) {
	r := gin.New()
	// 中间件顺序集中在这里维护，顺序本身就是契约，改动前先读 applyMiddleware 的注释。
	applyMiddleware(r, cfg)

	// gin 默认信任所有代理头。只有显式配置 TRUSTED_PROXIES 才信任，否则用 TCP 对端地址。
	if err := r.SetTrustedProxies(cfg.TrustedProxies); err != nil {
		slog.Warn("TRUSTED_PROXIES 配置非法，已忽略全部代理头", "value", cfg.TrustedProxies, "error", err)
		_ = r.SetTrustedProxies(nil)
	}
	if len(cfg.TrustedProxies) == 0 {
		slog.Info("未配置 TRUSTED_PROXIES，客户端 IP 取 TCP 对端地址（反向代理后需配置）")
	}

	box, err := secretbox.New(cfg.EncryptionKey)
	if err != nil {
		return nil, fmt.Errorf("初始化 secretbox 失败: %w", err)
	}
	// 未配置 ENCRYPTION_KEY 时使用内置固定密钥，等同于明文；不是启动错误，但必须让运维看到。
	if cfg.EncryptionKey == "" {
		slog.Warn("ENCRYPTION_KEY 未配置，敏感字段使用内置固定密钥加密，等同于明文；生产环境请显式配置")
	}

	// 站点时区：先用环境变量默认值，随后由库里的设置覆盖；可变引用，改设置后立即生效
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
	// 站点地址的回退值来自环境变量 SITE_URL；库里配置了就以库为准（读取在各处拼接时现做）。
	configSvc.SiteURLFallback = cfg.SiteURL
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

	// 恢复重启前未完成的确认重发链
	if _, err := confirmMgr.RecoverChains(); err != nil {
		slog.Error("恢复确认重发链失败，重启前的催办将不会继续", "error", err)
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
	logSvc := services.NewLogService(database.DB, cfg.SiteURL)
	if cfg.LogAutoPurgeDays > 0 {
		purgeAfter := time.Duration(cfg.LogAutoPurgeDays) * 24 * time.Hour
		if _, err := engine.AddPurgeCron(logSvc, purgeAfter); err != nil {
			slog.Error("注册日志自动清理任务失败", "purge_days", cfg.LogAutoPurgeDays, "error", err)
		}
	}

	// 自动清理 AI 草稿：固定保留 90 天，不提供开关
	aiSvc := services.NewAIService(database.DB, box, tz, reminderSvc, channelSvc)
	if _, err := engine.AddDailyJob("AI草稿清理", scheduler.DailyPurgeExpr, func() {
		n, err := aiSvc.PurgeActions(services.ActionRetention)
		if err != nil {
			slog.Error("自动清理 AI 草稿失败", "error", err)
		} else if n > 0 {
			slog.Info("自动清理 AI 草稿", "count", n)
		}
	}); err != nil {
		slog.Warn("注册 AI 草稿清理任务失败", "error", err)
	} else {
		slog.Info("已注册 AI 草稿自动清理", "retention_days", int(services.ActionRetention.Hours()/24))
	}

	tokenSvc := services.NewTokenService(database.DB, box)
	userSvc := services.NewUserService(database.DB)

	loginLimiter := middleware.NewLoginLimiter(
		cfg.LoginMaxAttempts,
		time.Duration(cfg.LoginLockoutMin)*time.Minute,
	)
	authHandler := &handlers.AuthHandler{JWTSecret: cfg.JWTSecret, UserSvc: userSvc, Limiter: loginLimiter}
	userHandler := &handlers.UserHandler{Svc: userSvc}
	channelHandler := &handlers.ChannelHandler{Svc: channelSvc}
	reminderHandler := &handlers.ReminderHandler{Svc: reminderSvc}
	logHandler := &handlers.LogHandler{Svc: logSvc}
	confirmHandler := &handlers.ConfirmHandler{Svc: confirmSvc, TZ: tz}
	tokenHandler := &handlers.TokenHandler{Svc: tokenSvc}
	externalHandler := &handlers.ExternalHandler{ReminderSvc: reminderSvc, TokenSvc: tokenSvc, ChannelSvc: channelSvc}
	schedulerHandler := &handlers.SchedulerHandler{Engine: engine, Sweeper: sweeper}
	configHandler := &handlers.ConfigHandler{Svc: configSvc}
	aiHandler := &handlers.AIHandler{Svc: aiSvc}

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

	protected := api.Group("")
	protected.Use(middleware.JWTAuth(cfg.JWTSecret))
	{
		channels := protected.Group("/channels")
		{
			channels.GET("", channelHandler.List)
			channels.POST("", channelHandler.Create)
			channels.GET("/:id", channelHandler.Get)
			channels.PUT("/:id", channelHandler.Update)
			channels.DELETE("/batch", channelHandler.BatchDelete) // 必须放在 /:id 之前
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

		ai := protected.Group("/ai")
		{
			ai.GET("/settings", aiHandler.GetSettings)
			ai.PUT("/settings", aiHandler.UpdateSettings)
			ai.POST("/test", aiHandler.Test)
			ai.POST("/models", aiHandler.FetchModels)
			ai.GET("/actions", aiHandler.ListActions)
			ai.POST("/chat", aiHandler.Chat)
			ai.POST("/chat/stream", aiHandler.ChatStream)
			ai.POST("/actions/:id/apply", aiHandler.ApplyAction)
			ai.POST("/actions/:id/reject", aiHandler.RejectAction)
		}

		cfgRoute := protected.Group("/config")
		cfgRoute.GET("", configHandler.GetAll)
		cfgRoute.PUT("", configHandler.Update)
	}

	serveStaticFiles(r, staticFS)
	registerNotFound(r)
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

// registerNotFound 给 /api 下未命中的路由补上统一错误体（其余路径返回纯文本 404）。
func registerNotFound(r *gin.Engine) {
	r.NoRoute(func(c *gin.Context) {
		if !strings.HasPrefix(c.Request.URL.Path, "/api") {
			// 必须自己写 body：只设状态码时 gin 仍会补上默认文本
			c.Data(http.StatusNotFound, "text/plain", []byte("404 page not found"))
			return
		}
		c.AbortWithStatusJSON(http.StatusNotFound,
			middleware.ErrorBody(middleware.CodeNotFound, "接口不存在", ""))
	})
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
