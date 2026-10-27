package handlers

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/reminder/backend/internal/middleware"
	"github.com/reminder/backend/internal/services"
)

// newConfigTestEngine 构造站点配置路由引擎。
func newConfigTestEngine(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := newTestDB(t)

	h := &ConfigHandler{Svc: services.NewConfigService(db, testTZ(cstZone()))}

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.ErrorHandler())
	r.GET("/config", h.GetAll)
	r.PUT("/config", h.Update)
	return r
}

// 初始为空 → 写入 → 读回 → 置空删除。
func TestConfigUpsertAndDelete(t *testing.T) {
	r := newConfigTestEngine(t)

	// 每次解码前重建变量：json.Unmarshal 对非 nil map 是"合并"语义，
	// 复用同一个变量会把上一次的 key 残留下来，导致误判。
	var getResp struct {
		Data map[string]string `json:"data"`
	}

	// 初始只有时区默认值（站点时区未在库里设置过时也会回显当前生效值）
	decodeJSON(t, doJSON(r, http.MethodGet, "/config", nil), &getResp)
	if len(getResp.Data) != 1 || getResp.Data["timezone"] == "" {
		t.Fatalf("初始应只有 timezone 默认值，实际 %v", getResp.Data)
	}

	// 写入两个 key
	w := doJSON(r, http.MethodPut, "/config", map[string]string{
		"site_name": "我的提醒",
		"theme":     "dark",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, body=%s", w.Code, w.Body.String())
	}
	getResp.Data = nil
	decodeJSON(t, w, &getResp)
	if getResp.Data["site_name"] != "我的提醒" || getResp.Data["theme"] != "dark" {
		t.Fatalf("写入后读取不一致: %v", getResp.Data)
	}

	// 覆盖已有 key
	getResp.Data = nil
	decodeJSON(t, doJSON(r, http.MethodPut, "/config", map[string]string{"theme": "light"}), &getResp)
	if getResp.Data["theme"] != "light" {
		t.Fatalf("覆盖失败: %v", getResp.Data)
	}

	// 置空应删除该 key
	getResp.Data = nil
	decodeJSON(t, doJSON(r, http.MethodPut, "/config", map[string]string{"theme": ""}), &getResp)
	if _, ok := getResp.Data["theme"]; ok {
		t.Fatalf("空值应删除 key，实际仍在: %v", getResp.Data)
	}
	if getResp.Data["site_name"] != "我的提醒" {
		t.Fatalf("其他 key 不应受影响: %v", getResp.Data)
	}
}

// 非法请求体应 400。
func TestConfigUpdate_InvalidBody(t *testing.T) {
	r := newConfigTestEngine(t)

	req := newRequest(http.MethodPut, "/config", nil)
	req.Body = http.NoBody
	w := serve(r, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("状态码 = %d, 期望 400", w.Code)
	}
}
