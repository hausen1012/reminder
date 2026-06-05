package router

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
	"strings"

	"github.com/bedrock/backend/internal/config"
	"github.com/bedrock/backend/internal/handlers"
	"github.com/bedrock/backend/internal/middleware"
	"github.com/gin-gonic/gin"
)

func Setup(staticFS embed.FS, cfg *config.Config) *gin.Engine {
	r := gin.Default()
	r.Use(middleware.ErrorHandler())
	r.Use(middleware.Logger())
	r.Use(middleware.CORS())

	authHandler := &handlers.AuthHandler{JWTSecret: cfg.JWTSecret}

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