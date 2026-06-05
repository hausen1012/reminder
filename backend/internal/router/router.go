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
	Engine  *scheduler.Engine
	Sweeper *scheduler.Sweeper
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
	reminderSvc := services.NewReminderService(database.DB, engine, cfg.Location, dispatchSvc)

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

	authHandler := &handlers.AuthHandler{JWTSecret: cfg.JWTSecret}
	channelHandler := &handlers.ChannelHandler{Svc: channelSvc}
	reminderHandler := &handlers.ReminderHandler{Svc: reminderSvc}

	api := r.Group("/api")
	{
		api.GET("/health", handlers.HealthCheck)
		api.POST("/auth/login", authHandler.Login)
	}

	auth := api.Group("/auth")
	auth.Use(middleware.JWTAuth(cfg.JWTSecret))
	{
		auth.GET("/me", authHandler.Me)
		auth.POST("/logout", authHandler.Logout)
		auth.PUT("/password", handlers.UpdatePassword)
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
		}

		reminders := protected.Group("/reminders")
		{
			reminders.GET("", reminderHandler.List)
			reminders.POST("", reminderHandler.Create)
			reminders.POST("/preview", reminderHandler.Preview)
			reminders.GET("/:id", reminderHandler.Get)
			reminders.PUT("/:id", reminderHandler.Update)
			reminders.DELETE("/:id", reminderHandler.Delete)
			reminders.PATCH("/:id/toggle", reminderHandler.Toggle)
			reminders.POST("/:id/test", reminderHandler.Test)
		}
	}

	serveStaticFiles(r, staticFS)
	return &SetupResult{
		Engine:  r,
		Handles: &SchedulerHandles{Engine: engine, Sweeper: sweeper},
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
