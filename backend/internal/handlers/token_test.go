package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/reminder/backend/internal/middleware"
	"github.com/reminder/backend/internal/services"
	"gorm.io/gorm"
)

// newTokenTestEngine 构造只含令牌路由的测试引擎。
func newTokenTestEngine(t *testing.T) *gin.Engine {
	r, _ := newTokenTestEngineWithDB(t)
	return r
}

// newTokenTestEngineWithDB 额外返回底层 DB，供需要造数/改数据的用例使用。
func newTokenTestEngineWithDB(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := newTestDB(t)

	h := &TokenHandler{Svc: services.NewTokenService(db)}

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.ErrorHandler())
	r.GET("/tokens", h.List)
	r.POST("/tokens", h.Create)
	r.GET("/tokens/stats", h.Stats)
	r.GET("/tokens/:id", h.Get)
	r.GET("/tokens/:id/plaintext", h.GetPlaintext)
	r.DELETE("/tokens/:id", h.Delete)
	r.PATCH("/tokens/:id/toggle", h.Toggle)
	r.PUT("/tokens/:id/channels", h.UpdateDefaultChannels)
	return r, db
}

// 创建令牌应一次性返回明文，且明文带 bdrk_ 前缀。
func TestTokenCreate_ReturnsPlaintextOnce(t *testing.T) {
	r := newTokenTestEngine(t)

	w := doJSON(r, http.MethodPost, "/tokens", map[string]any{"name": "测试令牌"})
	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, body=%s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			Plaintext string `json:"plaintext"`
			Key       struct {
				ID     uint   `json:"id"`
				Name   string `json:"name"`
				Prefix string `json:"prefix"`
			} `json:"key"`
		} `json:"data"`
	}
	decodeJSON(t, w, &resp)

	if resp.Data.Plaintext == "" {
		t.Fatal("创建时应返回明文")
	}
	if len(resp.Data.Plaintext) < 5 || resp.Data.Plaintext[:5] != "bdrk_" {
		t.Fatalf("明文应带 bdrk_ 前缀，实际 %q", resp.Data.Plaintext)
	}
	if resp.Data.Key.Name != "测试令牌" {
		t.Fatalf("name = %q", resp.Data.Key.Name)
	}
	// 列表视图中的前缀应是明文开头
	if resp.Data.Key.Prefix == "" || resp.Data.Key.Prefix != resp.Data.Plaintext[:len(resp.Data.Key.Prefix)] {
		t.Fatalf("prefix = %q 与明文不匹配", resp.Data.Key.Prefix)
	}
}

// 名称为空应返回 400。
func TestTokenCreate_EmptyName(t *testing.T) {
	r := newTokenTestEngine(t)
	w := doJSON(r, http.MethodPost, "/tokens", map[string]any{"name": "  "})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("状态码 = %d, 期望 400, body=%s", w.Code, w.Body.String())
	}
}

// 列表不应泄露完整明文（json tag 为 "-"）。
func TestTokenList_DoesNotLeakPlaintext(t *testing.T) {
	r := newTokenTestEngine(t)

	if w := doJSON(r, http.MethodPost, "/tokens", map[string]any{"name": "列表令牌"}); w.Code != http.StatusOK {
		t.Fatalf("创建失败: %d", w.Code)
	}

	w := doJSON(r, http.MethodGet, "/tokens", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("列表状态码 = %d", w.Code)
	}
	body := w.Body.String()
	if containsKey(body, "plaintext") {
		t.Fatalf("列表响应不应包含 plaintext 字段: %s", body)
	}
	if containsKey(body, "key_hash") {
		t.Fatalf("列表响应不应包含 key_hash 字段: %s", body)
	}
}

// 切换与删除。
func TestTokenToggleAndDelete(t *testing.T) {
	r := newTokenTestEngine(t)

	w := doJSON(r, http.MethodPost, "/tokens", map[string]any{"name": "切换令牌"})
	if w.Code != http.StatusOK {
		t.Fatalf("创建失败: %d", w.Code)
	}
	// 创建接口返回 {key:{id}, plaintext}，取 id 需走 key.id
	id := createdTokenID(t, w)

	w = doJSON(r, http.MethodPatch, "/tokens/"+itoa(id)+"/toggle", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("切换状态码 = %d", w.Code)
	}
	var toggleResp struct {
		Data struct {
			Enabled bool `json:"enabled"`
		} `json:"data"`
	}
	decodeJSON(t, w, &toggleResp)
	if toggleResp.Data.Enabled {
		t.Fatal("切换后应变为禁用")
	}

	if w = doJSON(r, http.MethodDelete, "/tokens/"+itoa(id), nil); w.Code != http.StatusOK {
		t.Fatalf("删除状态码 = %d", w.Code)
	}
	if w = doJSON(r, http.MethodGet, "/tokens/"+itoa(id), nil); w.Code != http.StatusNotFound {
		t.Fatalf("删除后读取应 404，实际 %d", w.Code)
	}
}

// 不存在/非法 id 的错误码。
func TestTokenNotFoundAndInvalidID(t *testing.T) {
	r := newTokenTestEngine(t)

	if w := doJSON(r, http.MethodGet, "/tokens/9999", nil); w.Code != http.StatusNotFound {
		t.Fatalf("不存在应 404，实际 %d", w.Code)
	}
	if w := doJSON(r, http.MethodGet, "/tokens/abc", nil); w.Code != http.StatusBadRequest {
		t.Fatalf("非法 id 应 400，实际 %d", w.Code)
	}
	if w := doJSON(r, http.MethodDelete, "/tokens/9999", nil); w.Code != http.StatusNotFound {
		t.Fatalf("删除不存在应 404，实际 %d", w.Code)
	}
}

// 更新默认通道：绑定后 Get 应能读回。
func TestTokenUpdateDefaultChannels(t *testing.T) {
	r := newTokenTestEngine(t)

	w := doJSON(r, http.MethodPost, "/tokens", map[string]any{
		"name":                "带通道",
		"default_channel_ids": []uint{1, 2},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("创建失败: %d", w.Code)
	}
	id := createdTokenID(t, w)

	w = doJSON(r, http.MethodGet, "/tokens/"+itoa(id), nil)
	var got struct {
		Data struct {
			DefaultChannelIDs []uint `json:"default_channel_ids"`
		} `json:"data"`
	}
	decodeJSON(t, w, &got)
	if len(got.Data.DefaultChannelIDs) != 2 {
		t.Fatalf("创建时应绑定 2 个通道，实际 %v", got.Data.DefaultChannelIDs)
	}

	// 替换为其它通道
	w = doJSON(r, http.MethodPut, "/tokens/"+itoa(id)+"/channels", map[string]any{
		"channel_ids": []uint{3},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("更新通道状态码 = %d", w.Code)
	}
	decodeJSON(t, doJSON(r, http.MethodGet, "/tokens/"+itoa(id), nil), &got)
	if len(got.Data.DefaultChannelIDs) != 1 || got.Data.DefaultChannelIDs[0] != 3 {
		t.Fatalf("通道应被替换为 [3]，实际 %v", got.Data.DefaultChannelIDs)
	}
}

// stats 接口应返回数组且含 usage_24h 字段。
func TestTokenStats(t *testing.T) {
	r := newTokenTestEngine(t)

	if w := doJSON(r, http.MethodPost, "/tokens", map[string]any{"name": "统计令牌"}); w.Code != http.StatusOK {
		t.Fatalf("创建失败: %d", w.Code)
	}

	w := doJSON(r, http.MethodGet, "/tokens/stats", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d", w.Code)
	}
	var resp struct {
		Data []struct {
			ID      uint  `json:"id"`
			Usage24 int64 `json:"usage_24h"`
		} `json:"data"`
	}
	decodeJSON(t, w, &resp)
	if len(resp.Data) != 1 {
		t.Fatalf("应返回 1 条统计，实际 %d", len(resp.Data))
	}
}

// containsKey 判断 JSON 响应体中是否出现某个字段名。
func containsKey(body, key string) bool {
	return strings.Contains(body, `"`+key+`"`)
}

// createdTokenID 从创建令牌的响应 {data:{key:{id}}} 中取出 id。
//
// 注意创建接口的返回结构与其它接口不同（多包了一层 key），
// 这是既有 API 契约，测试按实际结构解析。
func createdTokenID(t *testing.T, w *httptest.ResponseRecorder) uint {
	t.Helper()
	var resp struct {
		Data struct {
			Key struct {
				ID uint `json:"id"`
			} `json:"key"`
		} `json:"data"`
	}
	decodeJSON(t, w, &resp)
	if resp.Data.Key.ID == 0 {
		t.Fatalf("响应中缺少 key.id: %s", w.Body.String())
	}
	return resp.Data.Key.ID
}
