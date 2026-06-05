package router

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
	"strings"

	"github.com/bedrock/backend/internal/config"
	"github.com/bedrock/backend/internal/crypto/secretbox"
	"github.com/bedrock/backend/internal/database"
	"github.com/bedrock/backend/internal/handlers"
	"github.com/bedrock/backend/internal/middleware"
	"github.com/bedrock/backend/internal/services"
	"github.com/gin-gonic/gin"
)

func Setup(staticFS embed.FS, cfg *config.Config) *gin.Engine {
	r := gin.Default()
	r.Use(middleware.ErrorHandler())
	r.Use(middleware.Logger())
	r.Use(middleware.CORS())

	// 初始化加密 Box（失败时直接 panic：通道全部依赖它）
	box, err := secretbox.New(cfg.SecretBoxKey)
	if err != nil {
		log.Fatalf("初始化 secretbox 失败: %v", err)
	}

	authHandler := &handlers.AuthHandler{JWTSecret: cfg.JWTSecret}
	channelSvc := services.NewChannelService(database.DB, box)
	channelHandler := &handlers.ChannelHandler{Svc: channelSvc}

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

	// 业务资源接口需要登录
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
	}

	serveStaticFiles(r, staticFS)
	return r
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
