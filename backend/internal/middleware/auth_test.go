package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func newTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(ErrorHandler())
	return r
}

// 无 Authorization 头必须拒绝。
func TestJWTAuth_MissingHeader(t *testing.T) {
	r := newTestRouter()
	r.GET("/x", JWTAuth("secret"), func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("状态码 = %d, 期望 401", w.Code)
	}
}

// 非 Bearer 前缀必须拒绝。
func TestJWTAuth_WrongScheme(t *testing.T) {
	r := newTestRouter()
	r.GET("/x", JWTAuth("secret"), func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Basic abc")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("状态码 = %d, 期望 401", w.Code)
	}
}

// 用别的密钥签发的令牌必须拒绝。
func TestJWTAuth_WrongSecret(t *testing.T) {
	r := newTestRouter()
	r.GET("/x", JWTAuth("right-secret"), func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	token := signToken(t, "wrong-secret", time.Now().Add(time.Hour))
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("状态码 = %d, 期望 401", w.Code)
	}
}

// 已过期的令牌必须拒绝。
func TestJWTAuth_Expired(t *testing.T) {
	r := newTestRouter()
	r.GET("/x", JWTAuth("secret"), func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	token := signToken(t, "secret", time.Now().Add(-time.Hour))
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("状态码 = %d, 期望 401", w.Code)
	}
}

// 合法令牌应放行，并把 user_id / username 注入上下文。
func TestJWTAuth_Valid(t *testing.T) {
	r := newTestRouter()
	r.GET("/x", JWTAuth("secret"), func(c *gin.Context) {
		id, _ := c.Get("user_id")
		name, _ := c.Get("username")
		c.JSON(http.StatusOK, gin.H{"user_id": id, "username": name})
	})

	token := signToken(t, "secret", time.Now().Add(time.Hour))
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, 期望 200, body=%s", w.Code, w.Body.String())
	}

	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("响应不是合法 JSON: %v", err)
	}
	if got["username"] != "admin" {
		t.Fatalf("username = %v, 期望 admin", got["username"])
	}
	if got["user_id"] != float64(7) {
		t.Fatalf("user_id = %v, 期望 7", got["user_id"])
	}
}

func signToken(t *testing.T, secret string, exp time.Time) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id":  uint(7),
		"username": "admin",
		"exp":      exp.Unix(),
	})
	s, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("签发令牌失败: %v", err)
	}
	return s
}
