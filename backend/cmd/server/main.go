package main

import (
	"context"
	"embed"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/bedrock/backend/internal/config"
	"github.com/bedrock/backend/internal/database"
	"github.com/bedrock/backend/internal/router"
)

//go:embed web/*
var staticFS embed.FS

func main() {
	cfg := config.Load()

	// 日志同时输出到控制台和文件
	logDir := cfg.LogFile
	if logDir == "" {
		logDir = "logs"
	}
	if err := os.MkdirAll(logDir, 0755); err == nil {
		logPath := filepath.Join(logDir, fmt.Sprintf("app-%s.log", time.Now().Format("2006-01-02")))
		f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err == nil {
			log.SetOutput(io.MultiWriter(os.Stderr, f))
		}
	}

	if err := database.Init(cfg); err != nil {
		log.Fatalf("database init failed: %v", err)
	}

	res := router.Setup(staticFS, cfg)
	addr := fmt.Sprintf(":%s", cfg.Port)

	srv := &http.Server{
		Addr:    addr,
		Handler: res.Engine,
	}

	go func() {
		log.Printf("Server starting on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server start failed: %v", err)
		}
	}()

	// 优雅停机：先停 HTTP 接收新请求，再停调度器（确保不再有 fire 产生新事务）
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("收到退出信号，开始关闭…")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("HTTP 关闭失败: %v", err)
	}
	res.Handles.Sweeper.Stop()
	if res.Handles.ConfirmMgr != nil {
		res.Handles.ConfirmMgr.StopAll()
	}
	res.Handles.Engine.Stop()
	log.Println("已退出。")
}
