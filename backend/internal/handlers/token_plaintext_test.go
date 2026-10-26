package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// GetPlaintext：有明文时返回明文，缺明文时 404。
func TestTokenGetPlaintext(t *testing.T) {
	r := newTokenTestEngine(t)

	w := doJSON(r, http.MethodPost, "/tokens", map[string]any{"name": "明文令牌"})
	if w.Code != http.StatusOK {
		t.Fatalf("创建失败: %d", w.Code)
	}
	id := createdTokenID(t, w)
	plain := createdPlaintext(t, w)

	w = doJSON(r, http.MethodGet, "/tokens/"+itoa(id)+"/plaintext", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Data struct {
			Plaintext string `json:"plaintext"`
		} `json:"data"`
	}
	decodeJSON(t, w, &resp)
	if resp.Data.Plaintext != plain {
		t.Fatalf("明文不一致: 得到 %q，期望 %q", resp.Data.Plaintext, plain)
	}

	// 不存在的令牌应 404
	if w := doJSON(r, http.MethodGet, "/tokens/9999/plaintext", nil); w.Code != http.StatusNotFound {
		t.Fatalf("不存在应 404，实际 %d", w.Code)
	}
}

// 明文为空（历史数据）时应返回 404 并提示重新创建。
func TestTokenGetPlaintext_EmptyPlaintext(t *testing.T) {
	r, db := newTokenTestEngineWithDB(t)

	w := doJSON(r, http.MethodPost, "/tokens", map[string]any{"name": "清空明文"})
	if w.Code != http.StatusOK {
		t.Fatalf("创建失败: %d", w.Code)
	}
	id := createdTokenID(t, w)

	// 直接清空明文，模拟历史数据
	if err := db.Exec("UPDATE tokens SET plaintext = '' WHERE id = ?", id).Error; err != nil {
		t.Fatalf("清空明文失败: %v", err)
	}

	w = doJSON(r, http.MethodGet, "/tokens/"+itoa(id)+"/plaintext", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("明文为空应 404，实际 %d, body=%s", w.Code, w.Body.String())
	}
}

// 外部 API 文档页应返回 HTML 且包含鉴权说明。
func TestExternalDocs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/external/v1/docs", (&ExternalHandler{}).Docs)

	w := doJSON(r, http.MethodGet, "/api/external/v1/docs", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !containsSub(ct, "text/html") {
		t.Fatalf("Content-Type = %q，期望 text/html", ct)
	}
	body := w.Body.String()
	if !containsSub(body, "X-AUTH") {
		t.Fatalf("文档应说明 X-AUTH 鉴权方式: %s", truncate(body, 200))
	}
	if !containsSub(body, "/api/external/v1/reminders") {
		t.Fatal("文档应列出 reminders 端点")
	}
}

// createdPlaintext 取出创建令牌响应中的明文。
func createdPlaintext(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var resp struct {
		Data struct {
			Plaintext string `json:"plaintext"`
		} `json:"data"`
	}
	decodeJSON(t, w, &resp)
	if resp.Data.Plaintext == "" {
		t.Fatal("响应缺少 plaintext")
	}
	return resp.Data.Plaintext
}

// truncate 截断长字符串，便于错误信息可读。
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
