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

	// 初始只有时区默认值（未落库时也回显当前生效值）；
	// site_url 的回显依赖 SiteURLFallback，这里没配所以不出现
	decodeJSON(t, doJSON(r, http.MethodGet, "/config", nil), &getResp)
	if len(getResp.Data) != 1 || getResp.Data["timezone"] == "" {
		t.Fatalf("初始应只有 timezone 默认值，实际 %v", getResp.Data)
	}

	// 写入两个 key（都必须在站点配置白名单内）
	w := doJSON(r, http.MethodPut, "/config", map[string]string{
		"app_name": "我的提醒",
		"logo_svg": "<svg/>",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, body=%s", w.Code, w.Body.String())
	}
	getResp.Data = nil
	decodeJSON(t, w, &getResp)
	if getResp.Data["app_name"] != "我的提醒" || getResp.Data["logo_svg"] != "<svg/>" {
		t.Fatalf("写入后读取不一致: %v", getResp.Data)
	}

	// 覆盖已有 key
	getResp.Data = nil
	decodeJSON(t, doJSON(r, http.MethodPut, "/config", map[string]string{"app_name": "改名"}), &getResp)
	if getResp.Data["app_name"] != "改名" {
		t.Fatalf("覆盖失败: %v", getResp.Data)
	}

	// 置空应删除该 key
	getResp.Data = nil
	decodeJSON(t, doJSON(r, http.MethodPut, "/config", map[string]string{"logo_svg": ""}), &getResp)
	if _, ok := getResp.Data["logo_svg"]; ok {
		t.Fatalf("空值应删除 key，实际仍在: %v", getResp.Data)
	}
	if getResp.Data["app_name"] != "改名" {
		t.Fatalf("其他 key 不应受影响: %v", getResp.Data)
	}
}

// 白名单之外的 key 应 400，且不写入数据库。
//
// 这是防线而非便利性检查：通用配置接口若能写任意 key，
// 就可以绕过 /api/ai/settings 往 ai_api_key_enc 里塞明文。
func TestConfigUpdate_RejectsUnknownKey(t *testing.T) {
	r := newConfigTestEngine(t)

	w := doJSON(r, http.MethodPut, "/config", map[string]string{"ai_api_key_enc": "明文密钥"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("状态码 = %d, 期望 400, body=%s", w.Code, w.Body.String())
	}

	var resp struct {
		Error struct {
			Code  string `json:"code"`
			Field string `json:"field"`
		} `json:"error"`
	}
	decodeJSON(t, w, &resp)
	if resp.Error.Code != string(middleware.CodeValidationFailed) {
		t.Fatalf("error.code = %q", resp.Error.Code)
	}
	if resp.Error.Field != "ai_api_key_enc" {
		t.Fatalf("error.field = %q", resp.Error.Field)
	}

	// 拒绝之后该 key 不应出现在读取结果里（也不会落库）
	var getResp struct {
		Data map[string]string `json:"data"`
	}
	decodeJSON(t, doJSON(r, http.MethodGet, "/config", nil), &getResp)
	if _, ok := getResp.Data["ai_api_key_enc"]; ok {
		t.Fatal("非法 key 不应出现在配置里")
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
