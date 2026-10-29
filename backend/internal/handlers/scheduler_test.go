package handlers

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/reminder/backend/internal/middleware"
	"github.com/reminder/backend/internal/models"
	"github.com/reminder/backend/internal/scheduler"
)

// noopDispatcher 是 scheduler.Dispatcher 的空实现。
type noopDispatcher struct{}

func (noopDispatcher) Run(_ context.Context, _ *models.Reminder, _ uint) {}

// newTestScheduler 构造一个已启动的 engine 与 sweeper，供 health/status 测试复用。
func newTestScheduler(t *testing.T) (*scheduler.Engine, *scheduler.Sweeper) {
	t.Helper()
	db := newTestDB(t)
	loc := time.FixedZone("CST", 8*3600)

	engine := scheduler.NewEngine(db, noopDispatcher{}, testTZ(loc))
	engine.Start()
	t.Cleanup(engine.Stop)

	sweeper := scheduler.NewSweeper(db, engine, testTZ(loc), time.Minute, time.Hour, 30*time.Second)
	return engine, sweeper
}

// 调度器状态接口应返回 engine 与 sweeper 状态。
func TestSchedulerStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newTestDB(t)
	loc := time.FixedZone("CST", 8*3600)

	engine := scheduler.NewEngine(db, noopDispatcher{}, testTZ(loc))
	engine.Start()
	t.Cleanup(engine.Stop)

	sweeper := scheduler.NewSweeper(db, engine, testTZ(loc), time.Minute, time.Hour, 30*time.Second)
	h := &SchedulerHandler{Engine: engine, Sweeper: sweeper}

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.ErrorHandler())
	r.GET("/scheduler/status", h.Status)

	w := doJSON(r, http.MethodGet, "/scheduler/status", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, body=%s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			Engine struct {
				Running         bool `json:"running"`
				RegisteredCount int  `json:"registered_count"`
			} `json:"engine"`
			Sweeper struct {
				Running         bool `json:"running"`
				IntervalSeconds int  `json:"interval_seconds"`
			} `json:"sweeper"`
		} `json:"data"`
	}
	decodeJSON(t, w, &resp)
	if !resp.Data.Engine.Running {
		t.Fatal("engine 应为运行中")
	}
	if resp.Data.Sweeper.IntervalSeconds != 60 {
		t.Fatalf("interval_seconds = %d，期望 60", resp.Data.Sweeper.IntervalSeconds)
	}
}

// 调度器状态支持分页参数。
func TestSchedulerStatus_Paged(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newTestDB(t)
	loc := time.FixedZone("CST", 8*3600)

	engine := scheduler.NewEngine(db, noopDispatcher{}, testTZ(loc))
	engine.Start()
	t.Cleanup(engine.Stop)
	sweeper := scheduler.NewSweeper(db, engine, testTZ(loc), time.Minute, time.Hour, 30*time.Second)
	h := &SchedulerHandler{Engine: engine, Sweeper: sweeper}

	r := gin.New()
	r.GET("/scheduler/status", h.Status)

	if w := doJSON(r, http.MethodGet, "/scheduler/status?limit=10&offset=0", nil); w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d", w.Code)
	}
}
