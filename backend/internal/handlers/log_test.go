package handlers

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/reminder/backend/internal/middleware"
	"github.com/reminder/backend/internal/services"
	"gorm.io/gorm"
)

// newLogTestEngine 构造只含日志路由的测试引擎，并返回底层 DB 供造数。
func newLogTestEngine(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := newTestDB(t)

	h := &LogHandler{Svc: services.NewLogService(db, "http://example.test")}

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.ErrorHandler())
	r.GET("/logs", h.List)
	r.GET("/logs/count", h.PurgeCount)
	r.GET("/logs/:id", h.GetDetail)
	r.DELETE("/logs/batch", h.BatchDelete)
	r.DELETE("/logs", h.Purge)
	return r, db
}

type logListResp struct {
	Data struct {
		Items []struct {
			ID         uint   `json:"id"`
			RetryRound int    `json:"retry_round"`
			Title      string `json:"title"`
		} `json:"items"`
		Total   int64 `json:"total"`
		Count   int64 `json:"count"`
		Deleted int64 `json:"deleted"`
	} `json:"data"`
}

// 列表以 retry_round=0 的主记录作为行，链上重发跟随主记录展开。
func TestLogList_OnlyMainRowsCounted(t *testing.T) {
	r, db := newLogTestEngine(t)

	seedLog(t, db, "主记录", 0, nil)
	chain := "chain-x"
	mainWithChain := seedLog(t, db, "带确认链", 0, &chain)
	seedLog(t, db, "重发1", 1, &chain)
	seedLog(t, db, "重发2", 2, &chain)

	w := doJSON(r, http.MethodGet, "/logs", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, body=%s", w.Code, w.Body.String())
	}

	var resp logListResp
	decodeJSON(t, w, &resp)

	if resp.Data.Total != 2 {
		t.Fatalf("total = %d，期望 2（仅主记录）", resp.Data.Total)
	}
	if len(resp.Data.Items) == 0 {
		t.Fatal("items 不应为空")
	}
	if resp.Data.Items[0].ID != mainWithChain {
		t.Fatalf("首条应是 id=%d 的主记录，实际 %d", mainWithChain, resp.Data.Items[0].ID)
	}
}

// 按 status / search 过滤。
func TestLogList_Filters(t *testing.T) {
	r, db := newLogTestEngine(t)

	okLog := seedLog(t, db, "成功的日志", 0, nil)
	failedLog := seedLog(t, db, "失败的日志", 0, nil)
	if err := db.Exec("UPDATE delivery_logs SET status = ? WHERE id = ?", "failed", failedLog).Error; err != nil {
		t.Fatalf("更新状态失败: %v", err)
	}
	_ = okLog

	var resp logListResp

	decodeJSON(t, doJSON(r, http.MethodGet, "/logs?status=success", nil), &resp)
	if resp.Data.Total != 1 {
		t.Fatalf("status=success 应返回 1 条，实际 %d", resp.Data.Total)
	}

	decodeJSON(t, doJSON(r, http.MethodGet, "/logs?search=失败", nil), &resp)
	if resp.Data.Total != 1 {
		t.Fatalf("search=失败 应返回 1 条，实际 %d", resp.Data.Total)
	}
}

// 详情应返回 attempts。
func TestLogDetail_IncludesAttempts(t *testing.T) {
	r, db := newLogTestEngine(t)

	id := seedLog(t, db, "详情日志", 0, nil)
	seedAttempt(t, db, id, "success")

	w := doJSON(r, http.MethodGet, "/logs/"+itoa(id), nil)
	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d", w.Code)
	}

	var resp struct {
		Data struct {
			ID       uint `json:"id"`
			Attempts []struct {
				Status string `json:"status"`
			} `json:"attempts"`
		} `json:"data"`
	}
	decodeJSON(t, w, &resp)
	if len(resp.Data.Attempts) != 1 {
		t.Fatalf("应返回 1 条 attempt，实际 %d", len(resp.Data.Attempts))
	}
}

// 不存在/非法 id。
func TestLogDetail_Errors(t *testing.T) {
	r, _ := newLogTestEngine(t)

	if w := doJSON(r, http.MethodGet, "/logs/9999", nil); w.Code != http.StatusNotFound {
		t.Fatalf("不存在应 404，实际 %d", w.Code)
	}
	if w := doJSON(r, http.MethodGet, "/logs/abc", nil); w.Code != http.StatusBadRequest {
		t.Fatalf("非法 id 应 400，实际 %d", w.Code)
	}
}

// count 返回待清理条数，purge 真正删除。
func TestLogPurgeFlow(t *testing.T) {
	r, db := newLogTestEngine(t)

	for i := 0; i < 3; i++ {
		seedLog(t, db, "待清理", 0, nil)
	}

	var resp logListResp
	decodeJSON(t, doJSON(r, http.MethodGet, "/logs/count?all=true", nil), &resp)
	if resp.Data.Count != 3 {
		t.Fatalf("count = %d，期望 3", resp.Data.Count)
	}

	w := doJSON(r, http.MethodDelete, "/logs?all=true", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("purge 状态码 = %d", w.Code)
	}
	decodeJSON(t, w, &resp)
	if resp.Data.Deleted != 3 {
		t.Fatalf("deleted = %d，期望 3", resp.Data.Deleted)
	}

	decodeJSON(t, doJSON(r, http.MethodGet, "/logs", nil), &resp)
	if resp.Data.Total != 0 {
		t.Fatalf("清理后应无日志，实际 %d", resp.Data.Total)
	}
}

// 批量删除：空 ids 400，正常 ids 删除并连带清理 attempt。
func TestLogBatchDelete(t *testing.T) {
	r, db := newLogTestEngine(t)

	if w := doJSON(r, http.MethodDelete, "/logs/batch", map[string]any{"ids": []uint{}}); w.Code != http.StatusBadRequest {
		t.Fatalf("空 ids 应 400，实际 %d", w.Code)
	}

	id1 := seedLog(t, db, "删1", 0, nil)
	id2 := seedLog(t, db, "删2", 0, nil)
	seedAttempt(t, db, id1, "success")

	if w := doJSON(r, http.MethodDelete, "/logs/batch", map[string]any{"ids": []uint{id1, id2}}); w.Code != http.StatusOK {
		t.Fatalf("批量删除状态码 = %d", w.Code)
	}

	var resp logListResp
	decodeJSON(t, doJSON(r, http.MethodGet, "/logs", nil), &resp)
	if resp.Data.Total != 0 {
		t.Fatalf("删除后应无日志，实际 %d", resp.Data.Total)
	}

	// attempt 应被级联清理
	var attemptCount int64
	db.Table("delivery_attempts").Where("delivery_log_id = ?", id1).Count(&attemptCount)
	if attemptCount != 0 {
		t.Fatalf("删除日志后 attempt 未清理，残留 %d 条", attemptCount)
	}
}
