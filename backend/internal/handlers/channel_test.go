package handlers

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/reminder/backend/internal/crypto/secretbox"
	"github.com/reminder/backend/internal/middleware"
	"github.com/reminder/backend/internal/services"
)

// newChannelTestEngine 构造只含通道路由的测试引擎（暴露真实 CRUD 链路）。
func newChannelTestEngine(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := newTestDB(t)

	box, err := secretbox.New("")
	if err != nil {
		t.Fatalf("构造 secretbox 失败: %v", err)
	}
	h := &ChannelHandler{Svc: services.NewChannelService(db, box)}

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.ErrorHandler())
	r.GET("/channels", h.List)
	r.POST("/channels", h.Create)
	r.GET("/channels/:id", h.Get)
	r.PUT("/channels/:id", h.Update)
	r.DELETE("/channels/:id", h.Delete)
	r.DELETE("/channels/batch", h.BatchDelete)
	r.PATCH("/channels/:id/toggle", h.Toggle)
	r.POST("/channels/test-dry", h.TestDryRun)
	r.GET("/channels/stats", h.Stats)
	return r
}

// 创建 → 读取 → 列表 → 删除 的完整链路。
func TestChannelCRUD_Lifecycle(t *testing.T) {
	r := newChannelTestEngine(t)

	// 创建（config 必填，log 类型允许空对象）
	w := doJSON(r, http.MethodPost, "/channels", map[string]any{
		"name":   "测试通道",
		"type":   "log",
		"config": map[string]any{},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("创建状态码 = %d, body=%s", w.Code, w.Body.String())
	}
	id := dataID(t, w)

	// 读取
	w = doJSON(r, http.MethodGet, "/channels/"+itoa(id), nil)
	if w.Code != http.StatusOK {
		t.Fatalf("读取状态码 = %d", w.Code)
	}

	// 列表
	w = doJSON(r, http.MethodGet, "/channels", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("列表状态码 = %d", w.Code)
	}

	// 删除
	w = doJSON(r, http.MethodDelete, "/channels/"+itoa(id), nil)
	if w.Code != http.StatusOK {
		t.Fatalf("删除状态码 = %d", w.Code)
	}

	// 删除后再读应 404
	w = doJSON(r, http.MethodGet, "/channels/"+itoa(id), nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("删除后读取状态码 = %d, 期望 404", w.Code)
	}
}

// 非法 types 应被拒绝。
func TestChannelCreate_InvalidType(t *testing.T) {
	r := newChannelTestEngine(t)

	w := doJSON(r, http.MethodPost, "/channels", map[string]any{
		"name": "坏类型",
		"type": "telegram",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("状态码 = %d, 期望 400, body=%s", w.Code, w.Body.String())
	}
}

// 重名通道应返回 400。
func TestChannelCreate_DuplicateName(t *testing.T) {
	r := newChannelTestEngine(t)

	body := map[string]any{"name": "重名", "type": "log", "config": map[string]any{}}
	if w := doJSON(r, http.MethodPost, "/channels", body); w.Code != http.StatusOK {
		t.Fatalf("首次创建失败: %d", w.Code)
	}
	w := doJSON(r, http.MethodPost, "/channels", body)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("重名应返回 400，实际 %d, body=%s", w.Code, w.Body.String())
	}
}

// 非法 id 参数应返回 400。
func TestChannelGet_InvalidID(t *testing.T) {
	r := newChannelTestEngine(t)
	w := doJSON(r, http.MethodGet, "/channels/abc", nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("状态码 = %d, 期望 400", w.Code)
	}
}

// 批量删除空 ids 应返回 400。
func TestChannelBatchDelete_EmptyIDs(t *testing.T) {
	r := newChannelTestEngine(t)
	w := doJSON(r, http.MethodDelete, "/channels/batch", map[string]any{"ids": []uint{}})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("状态码 = %d, 期望 400", w.Code)
	}
}

// Toggle 应切换 enabled。
func TestChannelToggle(t *testing.T) {
	r := newChannelTestEngine(t)

	w := doJSON(r, http.MethodPost, "/channels", map[string]any{
		"name":   "切换",
		"type":   "log",
		"config": map[string]any{},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("创建失败: %d", w.Code)
	}
	id := dataID(t, w)

	w = doJSON(r, http.MethodPatch, "/channels/"+itoa(id)+"/toggle", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("切换状态码 = %d", w.Code)
	}
	var resp struct {
		Data struct {
			Enabled bool `json:"enabled"`
		} `json:"data"`
	}
	decodeJSON(t, w, &resp)
	if resp.Data.Enabled {
		t.Fatal("切换后 enabled 应为 false")
	}
}
