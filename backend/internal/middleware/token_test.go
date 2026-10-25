package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// 每分钟限额内应放行，超过后返回 429。
func TestTokenAuth_RateLimited(t *testing.T) {
	r := newTestRouter()
	verify := func(plain string) (uint, bool) { return 1, plain == "good" }
	r.GET("/x", TokenAuth(verify, nil, nil), func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	// 前 60 次应全部通过
	for i := 0; i < 60; i++ {
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.Header.Set("X-AUTH", "good")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("第 %d 次请求状态码 = %d, 期望 200", i+1, w.Code)
		}
	}

	// 第 61 次触发限流
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("X-AUTH", "good")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("状态码 = %d, 期望 429", w.Code)
	}
}

// 缺少 X-AUTH 头应返回 401。
func TestTokenAuth_MissingHeader(t *testing.T) {
	r := newTestRouter()
	verify := func(plain string) (uint, bool) { return 1, true }
	r.GET("/x", TokenAuth(verify, nil, nil), func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("状态码 = %d, 期望 401", w.Code)
	}
}

// 令牌无效应返回 401。
func TestTokenAuth_InvalidToken(t *testing.T) {
	r := newTestRouter()
	verify := func(plain string) (uint, bool) { return 0, false }
	r.GET("/x", TokenAuth(verify, nil, nil), func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("X-AUTH", "bad")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("状态码 = %d, 期望 401", w.Code)
	}
}

// 通过的请求应注入 token_id 并调用 toucher。
func TestTokenAuth_InjectsTokenID(t *testing.T) {
	r := newTestRouter()
	verify := func(plain string) (uint, bool) { return 42, true }

	var touched uint
	r.GET("/x", TokenAuth(verify, func(id uint) { touched = id }, nil), func(c *gin.Context) {
		id, _ := c.Get("token_id")
		c.JSON(http.StatusOK, gin.H{"token_id": id})
	})

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("X-AUTH", "good")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, 期望 200", w.Code)
	}
	if touched != 42 {
		t.Fatalf("toucher 收到 %d, 期望 42", touched)
	}
}
