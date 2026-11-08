package handlers

import (
	"encoding/json"
	"net/http"
	"slices"
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
	h := &ChannelHandler{Svc: services.NewChannelService(db, box, testTZ(cstZone()))}

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.ErrorHandler())
	r.GET("/channels", h.List)
	r.GET("/channels/types", h.Types)
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
		"type": "no-such-type",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("状态码 = %d, 期望 400, body=%s", w.Code, w.Body.String())
	}
}

// 类型元数据接口应返回全部类型，且每项都带字段定义。
func TestChannelTypes_ReturnsSpecs(t *testing.T) {
	r := newChannelTestEngine(t)

	w := doJSON(r, http.MethodGet, "/channels/types", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Data []struct {
			Type   string `json:"type"`
			Label  string `json:"label"`
			Fields []struct {
				Key  string `json:"key"`
				Kind string `json:"kind"`
			} `json:"fields"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	types := make([]string, 0, len(resp.Data))
	for _, s := range resp.Data {
		types = append(types, s.Type)
		if s.Label == "" {
			t.Errorf("类型 %q 缺少 label", s.Type)
		}
		if s.Type == "smtp" && len(s.Fields) == 0 {
			t.Error("smtp 应带有字段定义")
		}
	}
	for _, want := range []string{"smtp", "dingtalk", "wecom", "webhook", "log"} {
		if !slices.Contains(types, want) {
			t.Errorf("缺少类型 %q，实际 %v", want, types)
		}
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

// 通道试发：未知类型应 400。
func TestChannelTestDryRun(t *testing.T) {
	r := newChannelTestEngine(t)

	if w := doJSON(r, http.MethodPost, "/channels/test-dry", map[string]any{
		"type":   "telegram",
		"config": map[string]any{},
	}); w.Code != http.StatusBadRequest {
		t.Fatalf("未知类型应 400，实际 %d, body=%s", w.Code, w.Body.String())
	}
}
