package handlers

import (
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/reminder/backend/internal/crypto/secretbox"
	"github.com/reminder/backend/internal/middleware"
	"github.com/reminder/backend/internal/services"
)

// newAITestEngine 构造 AI 路由测试引擎。
func newAITestEngine(t *testing.T) (*gin.Engine, *services.AIService) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := newTestDB(t)
	box, err := secretbox.New("")
	if err != nil {
		t.Fatalf("构造 secretbox 失败: %v", err)
	}
	tz := testTZ(cstZone())
	channelSvc := services.NewChannelService(db, box, tz)
	dispatch := services.NewDispatchService(db, channelSvc, tz)
	reminderSvc := services.NewReminderService(db, nil, tz, dispatch, nil)
	svc := services.NewAIService(db, box, tz, reminderSvc, channelSvc)

	h := &AIHandler{Svc: svc}
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.ErrorHandler())
	r.GET("/ai/settings", h.GetSettings)
	r.PUT("/ai/settings", h.UpdateSettings)
	r.POST("/ai/models", h.FetchModels)
	r.GET("/ai/actions", h.ListActions)
	r.POST("/ai/chat", h.Chat)
	r.POST("/ai/chat/stream", h.ChatStream)
	r.POST("/ai/actions/:id/apply", h.ApplyAction)
	r.POST("/ai/actions/:id/reject", h.RejectAction)
	return r, svc
}

// 设置接口：保存后读回，密钥始终脱敏。
func TestAISettingsEndpoints(t *testing.T) {
	r, _ := newAITestEngine(t)

	var get struct {
		Data services.AISettings `json:"data"`
	}
	decodeJSON(t, doJSON(r, http.MethodGet, "/ai/settings", nil), &get)
	if get.Data.Provider != "anthropic" {
		t.Fatalf("默认 provider = %q", get.Data.Provider)
	}

	w := doJSON(r, http.MethodPut, "/ai/settings", services.AISettingsInput{
		Enabled:  true,
		Provider: "openai",
		Model:    "deepseek-chat",
		BaseURL:  "https://api.deepseek.com",
		APIKey:   "sk-abc",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, body=%s", w.Code, w.Body.String())
	}
	get.Data = services.AISettings{}
	decodeJSON(t, w, &get)
	if !get.Data.Enabled || get.Data.Model != "deepseek-chat" {
		t.Fatalf("保存结果不符: %+v", get.Data)
	}
	if get.Data.APIKey != "***" {
		t.Fatalf("密钥应脱敏为 ***，实际 %q", get.Data.APIKey)
	}
}

// 非法 provider / base_url / 无密钥启用都应 400。
func TestAISettingsValidation(t *testing.T) {
	r, _ := newAITestEngine(t)

	cases := []services.AISettingsInput{
		{Provider: "no-such", APIKey: "x"},
		{Provider: "anthropic", BaseURL: "ftp://x",
			APIKey: "x"},
		{Enabled: true, Provider: "anthropic"},
	}
	for i, in := range cases {
		w := doJSON(r, http.MethodPut, "/ai/settings", in)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("用例 %d 状态码 = %d，期望 400, body=%s", i, w.Code, w.Body.String())
		}
		var resp struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		decodeJSON(t, w, &resp)
		if resp.Error.Code != string(middleware.CodeValidationFailed) {
			t.Fatalf("用例 %d 错误码 = %q", i, resp.Error.Code)
		}
	}
}

// 自定义提示词随设置读写，且超长时 400。
func TestAISettingsCustomPrompt(t *testing.T) {
	r, _ := newAITestEngine(t)

	w := doJSON(r, http.MethodPut, "/ai/settings", services.AISettingsInput{
		Enabled:      true,
		Provider:     "openai",
		Model:        "deepseek-chat",
		APIKey:       "sk-abc",
		CustomPrompt: "回复请简洁，叫我老板",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, body=%s", w.Code, w.Body.String())
	}
	var put struct {
		Data services.AISettings `json:"data"`
	}
	decodeJSON(t, w, &put)
	if put.Data.CustomPrompt != "回复请简洁，叫我老板" {
		t.Fatalf("custom_prompt = %q", put.Data.CustomPrompt)
	}

	var get struct {
		Data services.AISettings `json:"data"`
	}
	decodeJSON(t, doJSON(r, http.MethodGet, "/ai/settings", nil), &get)
	if get.Data.CustomPrompt != "回复请简洁，叫我老板" {
		t.Fatalf("读回 custom_prompt = %q", get.Data.CustomPrompt)
	}

	w = doJSON(r, http.MethodPut, "/ai/settings", services.AISettingsInput{
		Provider:     "openai",
		APIKey:       "***",
		CustomPrompt: strings.Repeat("啊", 4001),
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("超长提示词状态码 = %d，期望 400", w.Code)
	}
}

// 流式对话：先推一条 thinking 状态；未启用时以 event: error 结束，且不再追加 JSON 错误体。
func TestAIChatStreamEmitsStatusAndError(t *testing.T) {
	r, _ := newAITestEngine(t)

	w := doJSON(r, http.MethodPost, "/ai/chat/stream", map[string]any{"message": "你好"})
	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, body=%s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("Content-Type = %q", ct)
	}
	body := w.Body.String()
	if !strings.Contains(body, "event: status") {
		t.Fatalf("应先推 thinking 状态: %s", body)
	}
	if !strings.Contains(body, "event: error") || !strings.Contains(body, "未启用") {
		t.Fatalf("未启用时应推 error 事件: %s", body)
	}
	if strings.Contains(body, `"code":200`) || strings.Contains(body, `"data":`) {
		t.Fatalf("流式响应不应再追加 JSON 成功体: %s", body)
	}
}

// 空消息属于业务校验，但此时已经开始推流，错误只能走 event: error。
func TestAIChatStreamValidationErrorAsEvent(t *testing.T) {
	r, _ := newAITestEngine(t)

	w := doJSON(r, http.MethodPost, "/ai/chat/stream", map[string]any{"message": "   "})
	if w.Code != http.StatusOK {
		t.Fatalf("推流后不应再改状态码，实际 %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "event: error") || !strings.Contains(body, "消息不能为空") {
		t.Fatalf("应推 error 事件: %s", body)
	}
}
func TestAIChatDisabledReturns400(t *testing.T) {
	r, _ := newAITestEngine(t)

	w := doJSON(r, http.MethodPost, "/ai/chat", map[string]any{"message": "你好"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("状态码 = %d, body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	decodeJSON(t, w, &resp)
	if resp.Error.Code != string(middleware.CodeValidationFailed) || resp.Error.Message == "" {
		t.Fatalf("错误体不符合约定: %+v", resp.Error)
	}
}

// 空消息 / 缺字段都应 400。
func TestAIChatInvalidBody(t *testing.T) {
	r, _ := newAITestEngine(t)
	w := doJSON(r, http.MethodPost, "/ai/chat", map[string]any{"message": "   "})
	if w.Code == http.StatusOK {
		t.Fatalf("空消息不应成功, body=%s", w.Body.String())
	}
}

// 操作不存在返回 404。
func TestAIActionNotFound(t *testing.T) {
	r, _ := newAITestEngine(t)

	w := doJSON(r, http.MethodPost, "/ai/actions/no-such/apply", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("状态码 = %d，期望 404", w.Code)
	}
	w = doJSON(r, http.MethodPost, "/ai/actions/no-such/reject", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("状态码 = %d，期望 404", w.Code)
	}
}

// 待确认列表为空时返回空数组，而不是 null（前端直接 map 不会崩）。
func TestAIListActionsEmpty(t *testing.T) {
	r, _ := newAITestEngine(t)

	w := doJSON(r, http.MethodGet, "/ai/actions?status=pending", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d", w.Code)
	}
	var resp struct {
		Data struct {
			Items []any `json:"items"`
		} `json:"data"`
	}
	decodeJSON(t, w, &resp)
	if resp.Data.Items == nil {
		t.Fatal("items 不应为 null")
	}
}

// 设置保存备选模型并在读取时回显。
func TestAISettingsModelsPersistence(t *testing.T) {
	r, _ := newAITestEngine(t)

	w := doJSON(r, http.MethodPut, "/ai/settings", services.AISettingsInput{
		Enabled:  true,
		Provider: "openai",
		Model:    "deepseek-chat",
		Models:   []string{"deepseek-chat", "deepseek-reasoner", "gpt-4o"},
		BaseURL:  "https://api.deepseek.com",
		APIKey:   "sk-abc",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("保存状态码 = %d", w.Code)
	}

	var get struct {
		Data services.AISettings `json:"data"`
	}
	decodeJSON(t, doJSON(r, http.MethodGet, "/ai/settings", nil), &get)
	if len(get.Data.Models) != 3 {
		t.Fatalf("期望 models 长度为 3，实际为 %d: %v", len(get.Data.Models), get.Data.Models)
	}
}
