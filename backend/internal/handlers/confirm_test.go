package handlers

import (
	"net/http"
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
		FiredAt:    models.TimeNow(),
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

	h := newConfirmHandler(svc)
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
		ReminderID: 1, FiredAt: models.TimeNow(), Title: "确认我", Status: "success", Source: "web",
	}
	if err := db.Create(dl).Error; err != nil {
		t.Fatalf("创建日志失败: %v", err)
	}
	token, _ := svc.CreateToken(dl.ID, time.Hour)

	h := newConfirmHandler(svc)
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

	// 重复点击应渲染幂等成功页（已确认过），而不是红色错误页
	w2 := doJSON(newConfirmEngine(h), http.MethodGet, "/c/"+token, nil)
	if w2.Code != http.StatusOK {
		t.Fatalf("重复确认状态码 = %d", w2.Code)
	}
	body2 := w2.Body.String()
	if containsSub(body2, "确认失败") {
		t.Fatalf("重复点击不应显示失败页: %s", body2)
	}
	if !containsSub(body2, "之前已确认") {
		t.Fatalf("重复点击应提示此前已确认: %s", body2)
	}
	// 确认时间按站点时区（用例注入东八区）展示，而不是 UTC
	wantTime := got.ConfirmedAt.Std().In(cstZone()).Format("2006-01-02 15:04:05")
	if !containsSub(body2, wantTime) {
		t.Fatalf("应展示站点时区下的首次确认时间 %s: %s", wantTime, body2)
	}
	// 反向校验：直接 Format（UTC）的文本不该出现，除非东西八区恰好相同
	if utc := got.ConfirmedAt.Std().UTC().Format("2006-01-02 15:04:05"); utc != wantTime && containsSub(body2, utc) {
		t.Fatalf("确认时间不应按 UTC 展示: %s", body2)
	}

	// 重复点击不应改变首次确认时间
	var after models.DeliveryLog
	if err := db.First(&after, dl.ID).Error; err != nil {
		t.Fatalf("读取日志失败: %v", err)
	}
	if after.ConfirmedAt == nil || !after.ConfirmedAt.Time.Equal(got.ConfirmedAt.Time) {
		t.Fatalf("重复点击改写了 confirmed_at: %v -> %v", got.ConfirmedAt, after.ConfirmedAt)
	}
}

// 无效 token 应返回错误页而非 500。
func TestConfirm_InvalidToken(t *testing.T) {
	db := newTestDB(t)
	h := newConfirmHandler(services.NewConfirmService(db, testConfig()))

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

	dl := &models.DeliveryLog{ReminderID: 1, FiredAt: models.TimeNow(), Title: "过期", Status: "success"}
	if err := db.Create(dl).Error; err != nil {
		t.Fatalf("创建日志失败: %v", err)
	}
	token, _ := svc.CreateToken(dl.ID, -time.Hour) // 已过期

	h := newConfirmHandler(svc)
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

// testConfig 返回测试用配置。
func testConfig() *config.Config {
	return &config.Config{SiteURL: "http://example.test"}
}

// newConfirmEngine 构造只挂确认路由的引擎。
func newConfirmEngine(h *ConfirmHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/c/:token", h.Confirm)
	return r
}

// newConfirmHandler 构造带固定站点时区的确认处理器。
//
// 确认页上的时间必须按站点时区展示，用例统一注入东八区，
// 避免结果随执行机器的本地时区漂移。
func newConfirmHandler(svc *services.ConfirmService) *ConfirmHandler {
	return &ConfirmHandler{Svc: svc, TZ: testTZ(cstZone())}
}
