package handlers

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"testing"

	"github.com/reminder/backend/internal/middleware"
)

// 通道列表：无参数走全量、有参数走分页，两者结构不同（前端据此区分）。
func TestChannelList_AllVsPaged(t *testing.T) {
	r := newChannelTestEngine(t)
	for i := 0; i < 3; i++ {
		if w := doJSON(r, http.MethodPost, "/channels", map[string]any{
			"name": "ch" + itoa(uint(i)), "type": "log", "config": map[string]any{},
		}); w.Code != http.StatusOK {
			t.Fatalf("创建失败: %d", w.Code)
		}
	}

	// 无参数：返回数组
	w := doJSON(r, http.MethodGet, "/channels", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d", w.Code)
	}
	var all []struct {
		ID uint `json:"id"`
	}
	var raw struct {
		Data []struct {
			ID uint `json:"id"`
		} `json:"data"`
	}
	decodeJSON(t, w, &raw)
	all = raw.Data
	if len(all) != 3 {
		t.Fatalf("全量应返回 3 条，实际 %d", len(all))
	}

	// 带 limit：返回 {items,total}
	w = doJSON(r, http.MethodGet, "/channels?limit=2&offset=0", nil)
	var paged struct {
		Data struct {
			Items []struct {
				ID uint `json:"id"`
			} `json:"items"`
			Total int64 `json:"total"`
		} `json:"data"`
	}
	decodeJSON(t, w, &paged)
	if paged.Data.Total != 3 || len(paged.Data.Items) != 2 {
		t.Fatalf("分页应为 total=3 len=2，实际 total=%d len=%d", paged.Data.Total, len(paged.Data.Items))
	}

	// enabled=false 过滤（新建默认启用，应为 0 条）
	w = doJSON(r, http.MethodGet, "/channels?enabled=false", nil)
	decodeJSON(t, w, &paged)
	if paged.Data.Total != 0 {
		t.Fatalf("enabled=false 应返回 0 条，实际 %d", paged.Data.Total)
	}
}

// 通道更新：成功路径与类型不可变更。
func TestChannelUpdate(t *testing.T) {
	r := newChannelTestEngine(t)

	w := doJSON(r, http.MethodPost, "/channels", map[string]any{
		"name": "待更新", "type": "log", "config": map[string]any{},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("创建失败: %d", w.Code)
	}
	id := dataID(t, w)

	// 改名字（类型保持不变）
	if w := doJSON(r, http.MethodPut, "/channels/"+itoa(id), map[string]any{
		"name": "改名后", "type": "log", "config": map[string]any{},
	}); w.Code != http.StatusOK {
		t.Fatalf("更新状态码 = %d, body=%s", w.Code, w.Body.String())
	}

	// 改类型应被拒绝
	w = doJSON(r, http.MethodPut, "/channels/"+itoa(id), map[string]any{
		"name": "改名后", "type": "smtp", "config": map[string]any{},
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("修改类型应 400，实际 %d, body=%s", w.Code, w.Body.String())
	}
}

// 通道统计接口应返回数组。
func TestChannelStats(t *testing.T) {
	r := newChannelTestEngine(t)

	if w := doJSON(r, http.MethodGet, "/channels/stats?window=24h", nil); w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, body=%s", w.Code, w.Body.String())
	}
}

// 通道批量删除：正常 ids 应成功。
func TestChannelBatchDelete_OK(t *testing.T) {
	r := newChannelTestEngine(t)

	var ids []uint
	for i := 0; i < 2; i++ {
		w := doJSON(r, http.MethodPost, "/channels", map[string]any{
			"name": "批删" + itoa(uint(i)), "type": "log", "config": map[string]any{},
		})
		if w.Code != http.StatusOK {
			t.Fatalf("创建失败: %d", w.Code)
		}
		ids = append(ids, dataID(t, w))
	}

	if w := doJSON(r, http.MethodDelete, "/channels/batch", map[string]any{"ids": ids}); w.Code != http.StatusOK {
		t.Fatalf("批量删除状态码 = %d", w.Code)
	}

	var resp struct {
		Data []struct {
			ID uint `json:"id"`
		} `json:"data"`
	}
	decodeJSON(t, doJSON(r, http.MethodGet, "/channels", nil), &resp)
	if len(resp.Data) != 0 {
		t.Fatalf("删除后应无通道，实际 %d", len(resp.Data))
	}
}

// 提醒列表：非法 limit/offset 应被容错（不报错）。
func TestReminderList_BadParamsTolerated(t *testing.T) {
	r, _ := newReminderTestEngine(t)

	if w := doJSON(r, http.MethodGet, "/reminders?limit=abc&offset=xyz", nil); w.Code != http.StatusOK {
		t.Fatalf("非法分页参数应容错，实际 %d", w.Code)
	}
	if w := doJSON(r, http.MethodGet, "/reminders?sort_by=id;DROP&sort_order=asc", nil); w.Code != http.StatusOK {
		t.Fatalf("非法排序应容错，实际 %d", w.Code)
	}
}

// 提醒 upcoming 的 within 非法值应回退为默认，不报错。
func TestReminderUpcoming_BadWithin(t *testing.T) {
	r, _ := newReminderTestEngine(t)

	if w := doJSON(r, http.MethodGet, "/reminders/upcoming?within=bad-value", nil); w.Code != http.StatusOK {
		t.Fatalf("非法 within 应容错，实际 %d", w.Code)
	}
}

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
