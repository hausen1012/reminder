package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/reminder/backend/internal/crypto/secretbox"
	"github.com/reminder/backend/internal/middleware"
	"github.com/reminder/backend/internal/models"
	"github.com/reminder/backend/internal/services"
	"gorm.io/gorm"
)

// externalTestStack 返回引擎、DB 与令牌服务（便于造令牌）。
type externalTestStack struct {
	engine   *gin.Engine
	db       *gorm.DB
	tokenSvc *services.TokenService
}

// newExternalTestEngine 构造外部 API v1 的测试引擎。
//
// 令牌鉴权用真实中间件，但通过 X-AUTH 直接注入明文令牌，
// 因此测试覆盖了"从令牌到 token_id 再到数据隔离"的完整链路。
func newExternalTestEngine(t *testing.T) *externalTestStack {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := newTestDB(t)

	loc := time.FixedZone("CST", 8*3600)
	box, err := secretbox.New("")
	if err != nil {
		t.Fatalf("构造 secretbox 失败: %v", err)
	}
	channelSvc := services.NewChannelService(db, box)
	dispatch := services.NewDispatchService(db, channelSvc, loc)
	dispatch.RetryDelays = []time.Duration{0}
	reminderSvc := services.NewReminderService(db, noopScheduler{}, loc, dispatch, nil)
	tokenSvc := services.NewTokenService(db)

	h := &ExternalHandler{
		ReminderSvc: reminderSvc,
		TokenSvc:    tokenSvc,
		ChannelSvc:  channelSvc,
	}

	verify := func(plain string) (uint, bool) {
		k, ok := tokenSvc.Verify(plain)
		if !ok || k == nil {
			return 0, false
		}
		return k.ID, true
	}

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.ErrorHandler())

	v1 := r.Group("/api/external/v1")
	v1.Use(middleware.TokenAuth(verify, nil, nil))
	{
		v1.POST("/reminders", h.CreateReminder)
		v1.GET("/reminders", h.ListReminders)
		v1.GET("/reminders/:id", h.GetReminder)
		v1.PUT("/reminders/:id", h.UpdateReminder)
		v1.DELETE("/reminders/:id", h.DeleteReminder)
		v1.GET("/channels", h.ListChannels)
	}
	return &externalTestStack{engine: r, db: db, tokenSvc: tokenSvc}
}

// doAuthed 发起带 X-AUTH 的请求。
func (s *externalTestStack) doAuthed(method, path, token string, body any) *httptest.ResponseRecorder {
	req := newRequest(method, path, body)
	req.Header.Set("X-AUTH", token)
	return serve(s.engine, req)
}

// 缺少 X-AUTH 时应 401。
func TestExternalApi_RequiresToken(t *testing.T) {
	stack := newExternalTestEngine(t)

	w := doJSON(stack.engine, http.MethodGet, "/api/external/v1/reminders", nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("状态码 = %d, 期望 401", w.Code)
	}
}

// 创建提醒应强制 source=api 并记录 token_id。
func TestExternalCreate_ForcesSourceAndToken(t *testing.T) {
	stack := newExternalTestEngine(t)
	plain, _, err := stack.tokenSvc.Create("外部令牌", nil)
	if err != nil {
		t.Fatalf("创建令牌失败: %v", err)
	}

	w := stack.doAuthed(http.MethodPost, "/api/external/v1/reminders", plain, map[string]any{
		"title":         "外部提醒",
		"calendar":      "solar",
		"schedule_type": "once",
		"schedule_spec": map[string]any{"at": "2099-01-01T09:00:00"},
		"source":        "web", // 应被强制覆盖为 api
	})
	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, body=%s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			ID      uint   `json:"id"`
			Source  string `json:"source"`
			TokenID *uint  `json:"token_id"`
		} `json:"data"`
	}
	decodeJSON(t, w, &resp)
	if resp.Data.Source != "api" {
		t.Fatalf("source = %q，应被强制为 api", resp.Data.Source)
	}
	if resp.Data.TokenID == nil || *resp.Data.TokenID == 0 {
		t.Fatal("应记录 token_id")
	}
}

// 未指定通道时应回退到令牌默认通道。
func TestExternalCreate_UsesTokenDefaultChannels(t *testing.T) {
	stack := newExternalTestEngine(t)

	// 先建一个真实通道
	ch := &models.Channel{
		Name: "log-ch", Type: "log", Enabled: true, Config: []byte("{}"),
	}
	if err := stack.db.Create(ch).Error; err != nil {
		t.Fatalf("创建通道失败: %v", err)
	}

	plain, key, err := stack.tokenSvc.Create("带默认通道", []uint{ch.ID})
	if err != nil {
		t.Fatalf("创建令牌失败: %v", err)
	}

	w := stack.doAuthed(http.MethodPost, "/api/external/v1/reminders", plain, map[string]any{
		"title":         "回退默认通道",
		"calendar":      "solar",
		"schedule_type": "once",
		"schedule_spec": map[string]any{"at": "2099-01-01T09:00:00"},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, body=%s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			ChannelIDs []uint `json:"channel_ids"`
		} `json:"data"`
	}
	decodeJSON(t, w, &resp)
	if len(resp.Data.ChannelIDs) != 1 || resp.Data.ChannelIDs[0] != ch.ID {
		t.Fatalf("应回退到令牌默认通道 [%d]，实际 %v", ch.ID, resp.Data.ChannelIDs)
	}
	_ = key
}

// 数据隔离：令牌 A 不能读取 / 修改 / 删除令牌 B 创建的提醒。
func TestExternalApi_TokenIsolation(t *testing.T) {
	stack := newExternalTestEngine(t)

	plainA, _, err := stack.tokenSvc.Create("令牌A", nil)
	if err != nil {
		t.Fatalf("创建令牌A失败: %v", err)
	}
	plainB, _, err := stack.tokenSvc.Create("令牌B", nil)
	if err != nil {
		t.Fatalf("创建令牌B失败: %v", err)
	}

	// A 创建一条提醒
	w := stack.doAuthed(http.MethodPost, "/api/external/v1/reminders", plainA, map[string]any{
		"title":         "A的提醒",
		"calendar":      "solar",
		"schedule_type": "once",
		"schedule_spec": map[string]any{"at": "2099-01-01T09:00:00"},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("A 创建失败: %d, body=%s", w.Code, w.Body.String())
	}
	id := dataID(t, w)

	// B 读取应 404
	if w := stack.doAuthed(http.MethodGet, "/api/external/v1/reminders/"+itoa(id), plainB, nil); w.Code != http.StatusNotFound {
		t.Fatalf("B 读取 A 的提醒应 404，实际 %d", w.Code)
	}

	// B 修改应 404
	if w := stack.doAuthed(http.MethodPut, "/api/external/v1/reminders/"+itoa(id), plainB, map[string]any{
		"title":         "被篡改",
		"calendar":      "solar",
		"schedule_type": "once",
		"schedule_spec": map[string]any{"at": "2099-01-01T09:00:00"},
	}); w.Code != http.StatusNotFound {
		t.Fatalf("B 修改 A 的提醒应 404，实际 %d", w.Code)
	}

	// B 删除应 404
	if w := stack.doAuthed(http.MethodDelete, "/api/external/v1/reminders/"+itoa(id), plainB, nil); w.Code != http.StatusNotFound {
		t.Fatalf("B 删除 A 的提醒应 404，实际 %d", w.Code)
	}

	// A 自己仍能正常读取
	if w := stack.doAuthed(http.MethodGet, "/api/external/v1/reminders/"+itoa(id), plainA, nil); w.Code != http.StatusOK {
		t.Fatalf("A 读取自己的提醒应 200，实际 %d", w.Code)
	}
}

// 列表只返回本令牌创建的提醒。
func TestExternalList_OnlyOwnReminders(t *testing.T) {
	stack := newExternalTestEngine(t)

	plainA, _, _ := stack.tokenSvc.Create("列表A", nil)
	plainB, _, _ := stack.tokenSvc.Create("列表B", nil)

	body := map[string]any{
		"title":         "A的提醒",
		"calendar":      "solar",
		"schedule_type": "once",
		"schedule_spec": map[string]any{"at": "2099-01-01T09:00:00"},
	}
	if w := stack.doAuthed(http.MethodPost, "/api/external/v1/reminders", plainA, body); w.Code != http.StatusOK {
		t.Fatalf("A 创建失败: %d", w.Code)
	}

	var resp struct {
		Data struct {
			Total int64 `json:"total"`
		} `json:"data"`
	}
	decodeJSON(t, stack.doAuthed(http.MethodGet, "/api/external/v1/reminders", plainA, nil), &resp)
	if resp.Data.Total != 1 {
		t.Fatalf("A 应看到 1 条，实际 %d", resp.Data.Total)
	}

	decodeJSON(t, stack.doAuthed(http.MethodGet, "/api/external/v1/reminders", plainB, nil), &resp)
	if resp.Data.Total != 0 {
		t.Fatalf("B 不应看到 A 的提醒，实际 %d 条", resp.Data.Total)
	}
}

// 无效令牌应 401。
func TestExternalApi_InvalidToken(t *testing.T) {
	stack := newExternalTestEngine(t)

	req := newRequest(http.MethodGet, "/api/external/v1/reminders", nil)
	req.Header.Set("X-AUTH", "bdrk_invalid_token_value")
	w := serve(stack.engine, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("状态码 = %d, 期望 401", w.Code)
	}
}

// 令牌被禁用后应立刻失效。
func TestExternalApi_DisabledTokenRejected(t *testing.T) {
	stack := newExternalTestEngine(t)

	plain, key, err := stack.tokenSvc.Create("待禁用", nil)
	if err != nil {
		t.Fatalf("创建令牌失败: %v", err)
	}
	if _, err := stack.tokenSvc.Toggle(key.ID); err != nil {
		t.Fatalf("禁用令牌失败: %v", err)
	}

	req := newRequest(http.MethodGet, "/api/external/v1/reminders", nil)
	req.Header.Set("X-AUTH", plain)
	if w := serve(stack.engine, req); w.Code != http.StatusUnauthorized {
		t.Fatalf("禁用后应 401，实际 %d", w.Code)
	}
}

// 列表通道接口应可用，且不泄露敏感字段。
func TestExternalListChannels(t *testing.T) {
	stack := newExternalTestEngine(t)

	plain, _, err := stack.tokenSvc.Create("通道令牌", nil)
	if err != nil {
		t.Fatalf("创建令牌失败: %v", err)
	}

	ch := &models.Channel{
		Name: "smtp-外发", Type: "smtp", Enabled: true,
		Config: []byte(`{"host":"smtp.test","port":587,"password_enc":"c2VjcmV0"}`),
	}
	if err := stack.db.Create(ch).Error; err != nil {
		t.Fatalf("创建通道失败: %v", err)
	}

	w := stack.doAuthed(http.MethodGet, "/api/external/v1/channels", plain, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d", w.Code)
	}
	body := w.Body.String()
	// 敏感字段应被脱敏为 ***，不应出现原始密文
	if strings.Contains(body, "c2VjcmV0") {
		t.Fatalf("响应泄露了加密字段原始密文: %s", body)
	}
	if !strings.Contains(body, "***") {
		t.Fatalf("敏感字段应脱敏为 ***，实际: %s", body)
	}
}
