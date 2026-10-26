package handlers

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/reminder/backend/internal/config"
	"github.com/reminder/backend/internal/models"
	"github.com/reminder/backend/internal/services"
)

// 确认链接成功页应包含提醒标题，并对 HTML 特殊字符转义（防 XSS）。
func TestConfirm_SuccessPageEscapesTitle(t *testing.T) {
	db := newTestDB(t)
	svc := services.NewConfirmService(db, testConfig())

	// 造一条日志 + 确认令牌
	dl := &models.DeliveryLog{
		ReminderID: 1,
		FiredAt:    time.Now(),
		Title:      `<script>alert("xss")</script>`,
		Content:    "正文",
		Status:     "success",
		Source:     "web",
	}
	if err := db.Create(dl).Error; err != nil {
		t.Fatalf("创建日志失败: %v", err)
	}
	token, err := svc.CreateToken(dl.ID, time.Hour)
	if err != nil {
		t.Fatalf("创建确认令牌失败: %v", err)
	}

	h := &ConfirmHandler{Svc: svc}
	r := newConfirmEngine(h)

	w := doJSON(r, http.MethodGet, "/c/"+token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d", w.Code)
	}
	body := w.Body.String()

	// 原始脚本标签不得出现（必须被转义）
	if containsKey(body, "script>alert") || containsSub(body, "<script>") {
		t.Fatalf("标题未经转义，存在 XSS 风险: %s", body)
	}
	if !containsSub(body, "&lt;script&gt;") {
		t.Fatalf("应转义为 &lt;script&gt;: %s", body)
	}
}

// 确认成功后应标记 confirmed 且 token 被消费。
func TestConfirm_MarksConfirmed(t *testing.T) {
	db := newTestDB(t)
	svc := services.NewConfirmService(db, testConfig())

	dl := &models.DeliveryLog{
		ReminderID: 1, FiredAt: time.Now(), Title: "确认我", Status: "success", Source: "web",
	}
	if err := db.Create(dl).Error; err != nil {
		t.Fatalf("创建日志失败: %v", err)
	}
	token, _ := svc.CreateToken(dl.ID, time.Hour)

	h := &ConfirmHandler{Svc: svc}
	w := doJSON(newConfirmEngine(h), http.MethodGet, "/c/"+token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d", w.Code)
	}

	var got models.DeliveryLog
	if err := db.First(&got, dl.ID).Error; err != nil {
		t.Fatalf("读取日志失败: %v", err)
	}
	if !got.Confirmed {
		t.Fatal("确认后应标记 confirmed")
	}

	var ct models.ConfirmToken
	db.Where("token = ?", token).First(&ct)
	if ct.UsedAt == nil {
		t.Fatal("确认后 token.used_at 应写入")
	}

	// 重复点击应提示已使用，且仍是 200 页面
	w2 := doJSON(newConfirmEngine(h), http.MethodGet, "/c/"+token, nil)
	if w2.Code != http.StatusOK {
		t.Fatalf("重复确认状态码 = %d", w2.Code)
	}
	if !containsSub(w2.Body.String(), "已被使用") {
		t.Fatalf("重复确认应提示已使用: %s", w2.Body.String())
	}
}

// 无效 token 应返回错误页而非 500。
func TestConfirm_InvalidToken(t *testing.T) {
	db := newTestDB(t)
	h := &ConfirmHandler{Svc: services.NewConfirmService(db, testConfig())}

	w := doJSON(newConfirmEngine(h), http.MethodGet, "/c/nonexistent-token", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d", w.Code)
	}
	if !containsSub(w.Body.String(), "确认失败") {
		t.Fatalf("应渲染确认失败页: %s", w.Body.String())
	}
}

// 过期 token 应返回"已过期"提示。
func TestConfirm_ExpiredToken(t *testing.T) {
	db := newTestDB(t)
	svc := services.NewConfirmService(db, testConfig())

	dl := &models.DeliveryLog{ReminderID: 1, FiredAt: time.Now(), Title: "过期", Status: "success"}
	if err := db.Create(dl).Error; err != nil {
		t.Fatalf("创建日志失败: %v", err)
	}
	token, _ := svc.CreateToken(dl.ID, -time.Hour) // 已过期

	h := &ConfirmHandler{Svc: svc}
	w := doJSON(newConfirmEngine(h), http.MethodGet, "/c/"+token, nil)
	if !containsSub(w.Body.String(), "已过期") {
		t.Fatalf("应提示已过期: %s", w.Body.String())
	}
}

// escapeHTML 应转义全部五个特殊字符。
func TestEscapeHTML(t *testing.T) {
	cases := map[string]string{
		"&":  "&amp;",
		"<":  "&lt;",
		">":  "&gt;",
		`"`:  "&quot;",
		"'":  "&#39;",
		"普通": "普通",
	}
	for in, want := range cases {
		if got := escapeHTML(in); got != want {
			t.Errorf("escapeHTML(%q) = %q, 期望 %q", in, got, want)
		}
	}
}

// 组合输入应被完整转义。
func TestEscapeHTML_Combined(t *testing.T) {
	in := `<a href="x">'&'</a>`
	got := escapeHTML(in)
	want := `&lt;a href=&quot;x&quot;&gt;&#39;&amp;&#39;&lt;/a&gt;`
	if got != want {
		t.Fatalf("escapeHTML(%q) = %q, 期望 %q", in, got, want)
	}
}

// containsSub 判断字符串是否包含子串。
func containsSub(s, sub string) bool {
	return strings.Contains(s, sub)
}

// testConfig 返回测试用配置。
func testConfig() *config.Config {
	return &config.Config{BaseURL: "http://example.test"}
}

// newConfirmEngine 构造只挂确认路由的引擎。
func newConfirmEngine(h *ConfirmHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/c/:token", h.Confirm)
	return r
}
