package main

import (
	"context"
	"embed"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/reminder/backend/internal/config"
	"github.com/reminder/backend/internal/database"
	"github.com/reminder/backend/internal/logger"
	"github.com/reminder/backend/internal/router"
)

//go:embed web/*
var staticFS embed.FS

func main() {
	// 先解析配置再装配日志，确保日志按配置的站点时区按日滚动。
	cfg := config.Load()

	logger.Init(cfg.LogFile)

	slog.Info("启动配置",
		"port", cfg.Port,
		"db_path", cfg.DBPath,
		"site_url", cfg.SiteURL,
		"timezone", cfg.Timezone,
		"log_file", cfg.LogFile,
		"log_format", getEnv("LOG_FORMAT", "text"),
		"log_level", getEnv("LOG_LEVEL", "info"),
		"sweep_interval_sec", cfg.SweepIntervalSec,
		"miss_tolerance_minutes", cfg.MissToleranceMinutes,
		"log_auto_purge_days", cfg.LogAutoPurgeDays,
	)

	if err := database.Init(cfg); err != nil {
		slog.Error("数据库初始化失败", "db_path", cfg.DBPath, "error", err)
		os.Exit(1)
	}

	res, err := router.Setup(staticFS, cfg)
	if err != nil {
		slog.Error("路由初始化失败", "error", err)
		os.Exit(1)
	}
	addr := ":" + cfg.Port

	srv := &http.Server{
		Addr:              addr,
		Handler:           res.Engine,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		slog.Info("服务启动", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("服务启动失败", "error", err)
			os.Exit(1)
		}
	}()

	// 优雅停机
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	slog.Info("收到退出信号，开始关闭…")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("HTTP 关闭失败", "error", err)
	}
	res.Handles.Sweeper.Stop()
	if res.Handles.ConfirmMgr != nil {
		res.Handles.ConfirmMgr.StopAll()
	}
	res.Handles.Engine.Stop()
	slog.Info("已退出。")
}

// getEnv 读取环境变量，未设置或为空时返回 fallback。
func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
