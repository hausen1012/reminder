package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/reminder/backend/internal/middleware"
	"github.com/reminder/backend/internal/services"
)

// newAboutTestEngine 构造关于页路由引擎，接口指向给定的假服务。
func newAboutTestEngine(t *testing.T, apiURL string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	svc := services.NewAboutService(newTestDB(t), "v1.4.1", "2026-11-01T08:00:00Z")
	svc.APIURL = apiURL
	h := &AboutHandler{Svc: svc}

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.ErrorHandler())
	r.GET("/about", h.Get)
	r.POST("/about/check", h.Check)
	r.PUT("/about/update-check", h.UpdateCheck)
	return r
}

// fakeTagsServer 返回固定标签列表的假 GitHub 接口。
func fakeTagsServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// 首次读取：包含版本、构建时间与默认开启的检查状态。
func TestAboutGet_InitialState(t *testing.T) {
	r := newAboutTestEngine(t, "http://127.0.0.1:1")

	var resp struct {
		Data services.AboutInfo `json:"data"`
	}
	decodeJSON(t, doJSON(r, http.MethodGet, "/about", nil), &resp)
	if resp.Data.Version != "v1.4.1" || resp.Data.BuildTime != "2026-11-01T08:00:00Z" {
		t.Fatalf("版本信息不符: %+v", resp.Data)
	}
	if !resp.Data.Update.Enabled {
		t.Fatal("默认应开启自动检查")
	}
	if resp.Data.Update.LatestVersion != "" || resp.Data.Update.UpdateAvailable {
		t.Fatalf("未检查时不应有结论: %+v", resp.Data.Update)
	}
}

// 手动检查：成功后返回最新版本与是否有更新。
func TestAboutCheck_ReturnsLatestVersion(t *testing.T) {
	srv := fakeTagsServer(t, `[{"name":"v1.4.0"},{"name":"v1.5.0"}]`)
	r := newAboutTestEngine(t, srv.URL)

	w := doJSON(r, http.MethodPost, "/about/check", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Data services.AboutInfo `json:"data"`
	}
	decodeJSON(t, w, &resp)
	if resp.Data.Update.LatestVersion != "v1.5.0" || !resp.Data.Update.UpdateAvailable {
		t.Fatalf("检查结果不符: %+v", resp.Data.Update)
	}
}

// 检查接口不可达时返回 200，但状态里带 error。
func TestAboutCheck_FailureReportedInBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	r := newAboutTestEngine(t, srv.URL)

	w := doJSON(r, http.MethodPost, "/about/check", nil)
	var resp struct {
		Data services.AboutInfo `json:"data"`
	}
	decodeJSON(t, w, &resp)
	if resp.Data.Update.Error == "" {
		t.Fatal("失败时应带 error")
	}
}

// 开关接口：写入后读取应反映新状态。
func TestAboutUpdateCheck_TogglesSwitch(t *testing.T) {
	r := newAboutTestEngine(t, "http://127.0.0.1:1")

	w := doJSON(r, http.MethodPut, "/about/update-check", map[string]bool{"enabled": false})
	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Data services.AboutInfo `json:"data"`
	}
	decodeJSON(t, w, &resp)
	if resp.Data.Update.Enabled {
		t.Fatal("关闭后应为关闭")
	}

	resp.Data = services.AboutInfo{}
	decodeJSON(t, doJSON(r, http.MethodGet, "/about", nil), &resp)
	if resp.Data.Update.Enabled {
		t.Fatal("再次读取仍应为关闭")
	}
}

// 缺少 enabled 字段应 400。
func TestAboutUpdateCheck_MissingField(t *testing.T) {
	r := newAboutTestEngine(t, "http://127.0.0.1:1")

	w := doJSON(r, http.MethodPut, "/about/update-check", map[string]string{})
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
	if resp.Error.Code != string(middleware.CodeValidationFailed) || resp.Error.Field != "enabled" {
		t.Fatalf("错误体不符: %+v", resp.Error)
	}
}
