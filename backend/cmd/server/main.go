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
	cfg := config.Load()

	logger.Init(cfg.LogFile)

	if err := database.Init(cfg); err != nil {
		slog.Error("数据库初始化失败", "error", err)
		os.Exit(1)
	}

	res := router.Setup(staticFS, cfg)
	addr := ":" + cfg.Port

	srv := &http.Server{
		Addr:    addr,
		Handler: res.Engine,
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