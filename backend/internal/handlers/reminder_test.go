package handlers

import (
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/reminder/backend/internal/crypto/secretbox"
	"github.com/reminder/backend/internal/middleware"
	"github.com/reminder/backend/internal/models"
	"github.com/reminder/backend/internal/services"
	"gorm.io/gorm"
)

// newReminderTestEngine 构造只含提醒路由的测试引擎。
//
// scheduler hook 用 noop 替代，避免测试中启动真实 cron。
func newReminderTestEngine(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := newTestDB(t)

	loc := time.FixedZone("CST", 8*3600)
	tz := testTZ(loc)
	box, err := secretbox.New("")
	if err != nil {
		t.Fatalf("构造 secretbox 失败: %v", err)
	}
	channelSvc := services.NewChannelService(db, box, tz)
	dispatch := services.NewDispatchService(db, channelSvc, tz)
	dispatch.RetryDelays = []time.Duration{0}

	reminderSvc := services.NewReminderService(db, noopScheduler{}, tz, dispatch, nil)
	h := &ReminderHandler{Svc: reminderSvc}

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.ErrorHandler())
	r.GET("/reminders", h.List)
	r.POST("/reminders", h.Create)
	r.POST("/reminders/preview", h.Preview)
	r.GET("/reminders/upcoming", h.Upcoming)
	r.POST("/reminders/test-dry", h.TestDryRun)
	r.DELETE("/reminders/batch", h.BatchDelete)
	r.GET("/reminders/:id", h.Get)
	r.PUT("/reminders/:id", h.Update)
	r.DELETE("/reminders/:id", h.Delete)
	r.PATCH("/reminders/:id/toggle", h.Toggle)
	return r, db
}

// noopScheduler 是 services.SchedulerHook 的空实现，避免测试启动真实调度。
type noopScheduler struct{}

func (noopScheduler) Add(_ *models.Reminder) error    { return nil }
func (noopScheduler) Update(_ *models.Reminder) error { return nil }
func (noopScheduler) Remove(_ uint)                   {}

// 最小合法入参。
func onceBody(at string) map[string]any {
	return map[string]any{
		"title":         "测试提醒",
		"content":       "正文",
		"calendar":      "solar",
		"schedule_type": "once",
		"schedule_spec": map[string]any{"at": at},
	}
}

// CRUD 全链路。
func TestReminderCRUD_Lifecycle(t *testing.T) {
	r, _ := newReminderTestEngine(t)

	w := doJSON(r, http.MethodPost, "/reminders", onceBody("2099-01-01T09:00:00"))
	if w.Code != http.StatusOK {
		t.Fatalf("创建状态码 = %d, body=%s", w.Code, w.Body.String())
	}
	id := dataID(t, w)

	if w = doJSON(r, http.MethodGet, "/reminders/"+itoa(id), nil); w.Code != http.StatusOK {
		t.Fatalf("读取状态码 = %d", w.Code)
	}

	if w = doJSON(r, http.MethodPut, "/reminders/"+itoa(id), map[string]any{
		"title":         "改后标题",
		"calendar":      "solar",
		"schedule_type": "once",
		"schedule_spec": map[string]any{"at": "2099-02-01T10:00:00"},
	}); w.Code != http.StatusOK {
		t.Fatalf("更新状态码 = %d, body=%s", w.Code, w.Body.String())
	}

	if w = doJSON(r, http.MethodPatch, "/reminders/"+itoa(id)+"/toggle", nil); w.Code != http.StatusOK {
		t.Fatalf("切换状态码 = %d", w.Code)
	}

	if w = doJSON(r, http.MethodDelete, "/reminders/"+itoa(id), nil); w.Code != http.StatusOK {
		t.Fatalf("删除状态码 = %d", w.Code)
	}
	if w = doJSON(r, http.MethodGet, "/reminders/"+itoa(id), nil); w.Code != http.StatusNotFound {
		t.Fatalf("删除后应 404，实际 %d", w.Code)
	}
}

// 校验失败应 400。
func TestReminderCreate_Validation(t *testing.T) {
	r, _ := newReminderTestEngine(t)

	cases := []struct {
		name string
		body map[string]any
	}{
		{"标题为空", map[string]any{"title": "", "calendar": "solar", "schedule_type": "once", "schedule_spec": map[string]any{"at": "2099-01-01T09:00:00"}}},
		{"calendar 非法", map[string]any{"title": "x", "calendar": "mayan", "schedule_type": "once", "schedule_spec": map[string]any{"at": "2099-01-01T09:00:00"}}},
		{"schedule_type 非法", map[string]any{"title": "x", "calendar": "solar", "schedule_type": "weekly", "schedule_spec": map[string]any{"at": "2099-01-01T09:00:00"}}},
		{"缺 schedule_spec", map[string]any{"title": "x", "calendar": "solar", "schedule_type": "once"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := doJSON(r, http.MethodPost, "/reminders", tc.body)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("状态码 = %d, 期望 400, body=%s", w.Code, w.Body.String())
			}
		})
	}
}

// 预览接口应返回若干次触发时间。
func TestReminderPreview(t *testing.T) {
	r, _ := newReminderTestEngine(t)

	w := doJSON(r, http.MethodPost, "/reminders/preview", map[string]any{
		"calendar":      "solar",
		"schedule_type": "cron",
		"schedule_spec": map[string]any{"expr": "0 9 * * *"},
		"count":         3,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, body=%s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			Times []string `json:"times"`
		} `json:"data"`
	}
	decodeJSON(t, w, &resp)
	if len(resp.Data.Times) != 3 {
		t.Fatalf("应返回 3 个时间，实际 %d", len(resp.Data.Times))
	}
}

// 列表分页与过滤。
func TestReminderList_Paging(t *testing.T) {
	r, _ := newReminderTestEngine(t)

	for i := 0; i < 3; i++ {
		body := onceBody("2099-01-01T09:00:00")
		body["title"] = "提醒" + itoa(uint(i))
		if w := doJSON(r, http.MethodPost, "/reminders", body); w.Code != http.StatusOK {
			t.Fatalf("创建失败: %d", w.Code)
		}
	}

	w := doJSON(r, http.MethodGet, "/reminders?limit=2&offset=0", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d", w.Code)
	}
	var resp struct {
		Data struct {
			Items []struct {
				ID uint `json:"id"`
			} `json:"items"`
			Total int64 `json:"total"`
		} `json:"data"`
	}
	decodeJSON(t, w, &resp)
	if resp.Data.Total != 3 {
		t.Fatalf("total = %d，期望 3", resp.Data.Total)
	}
	if len(resp.Data.Items) != 2 {
		t.Fatalf("limit=2 应返回 2 条，实际 %d", len(resp.Data.Items))
	}
}

// upcoming：scope=today 与 within 参数都应可用。
func TestReminderUpcoming(t *testing.T) {
	r, _ := newReminderTestEngine(t)

	// 造一条今天稍后的提醒（若当前时间已晚，改用 within 校验不报错即可）
	body := onceBody(time.Now().Add(2 * time.Hour).Format("2006-01-02T15:04:05"))
	if w := doJSON(r, http.MethodPost, "/reminders", body); w.Code != http.StatusOK {
		t.Fatalf("创建失败: %d", w.Code)
	}

	if w := doJSON(r, http.MethodGet, "/reminders/upcoming?within=72h&limit=5", nil); w.Code != http.StatusOK {
		t.Fatalf("within 查询状态码 = %d", w.Code)
	}
	if w := doJSON(r, http.MethodGet, "/reminders/upcoming?scope=today", nil); w.Code != http.StatusOK {
		t.Fatalf("scope=today 状态码 = %d", w.Code)
	}
}

// 批量删除：空 ids 400；正常 ids 删除。
func TestReminderBatchDelete(t *testing.T) {
	r, _ := newReminderTestEngine(t)

	if w := doJSON(r, http.MethodDelete, "/reminders/batch", map[string]any{"ids": []uint{}}); w.Code != http.StatusBadRequest {
		t.Fatalf("空 ids 应 400，实际 %d", w.Code)
	}

	var ids []uint
	for i := 0; i < 2; i++ {
		body := onceBody("2099-01-01T09:00:00")
		body["title"] = "批删" + itoa(uint(i))
		w := doJSON(r, http.MethodPost, "/reminders", body)
		if w.Code != http.StatusOK {
			t.Fatalf("创建失败: %d", w.Code)
		}
		ids = append(ids, dataID(t, w))
	}

	if w := doJSON(r, http.MethodDelete, "/reminders/batch", map[string]any{"ids": ids}); w.Code != http.StatusOK {
		t.Fatalf("批量删除状态码 = %d", w.Code)
	}
}
