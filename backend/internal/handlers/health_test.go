package handlers

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/reminder/backend/internal/middleware"
)

// 健康检查：应返回 db/scheduler/sweeper 状态。
func TestHealthCheck(t *testing.T) {
	engine, sweeper := newTestScheduler(t)
	db := newTestDB(t)

	h := &HealthHandler{DB: db, Engine: engine, Sweeper: sweeper}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.ErrorHandler())
	r.GET("/health", h.Check)

	w := doJSON(r, http.MethodGet, "/health", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, body=%s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			Status  string `json:"status"`
			DBAlive bool   `json:"db_alive"`
			Sweeper struct {
				Running bool `json:"running"`
			} `json:"sweeper"`
		} `json:"data"`
	}
	decodeJSON(t, w, &resp)
	if resp.Data.Status != "healthy" {
		t.Fatalf("status = %q", resp.Data.Status)
	}
	if !resp.Data.DBAlive {
		t.Fatal("db_alive 应为 true")
	}
}
