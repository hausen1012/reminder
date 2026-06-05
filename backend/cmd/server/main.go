package main

import (
	"embed"
	"fmt"
	"log"

	"github.com/bedrock/backend/internal/config"
	"github.com/bedrock/backend/internal/database"
	"github.com/bedrock/backend/internal/router"
)

//go:embed web/*
var staticFS embed.FS

func main() {
	cfg := config.Load()

	if err := database.Init(cfg); err != nil {
		log.Fatalf("database init failed: %v", err)
	}

	r := router.Setup(staticFS, cfg)
	addr := fmt.Sprintf(":%s", cfg.Port)
	log.Printf("Server starting on %s", addr)
	if err := r.Run(addr); err != nil {
		log.Fatalf("server start failed: %v", err)
	}
}
