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

// alg=none 攻击：伪造一个无签名令牌，必须被拒绝。
func TestJWTAuth_RejectsNoneAlgorithm(t *testing.T) {
	r := newTestRouter()
	r.GET("/x", JWTAuth("secret"), func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	// 手工拼一个 alg=none 的令牌
	forged := "eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0." +
		"eyJ1c2VyX2lkIjoxLCJ1c2VybmFtZSI6ImhhY2tlciIsImV4cCI6OTk5OTk5OTk5OX0."

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+forged)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("alg=none 令牌应被拒绝，实际 %d, body=%s", w.Code, w.Body.String())
	}
}

// 用非 HMAC 算法（这里用 HS512 以外的合法算法不可签，故用篡改头部测试）：
// 头部声明 RS256 但用 HMAC 签名，必须被拒绝。
func TestJWTAuth_RejectsAlgorithmConfusion(t *testing.T) {
	r := newTestRouter()
	r.GET("/x", JWTAuth("secret"), func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	// 头部 alg=RS256，实际无有效签名
	forged := "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9." +
		"eyJ1c2VyX2lkIjoxLCJleHAiOjk5OTk5OTk5OTl9." +
		"ZmFrZXNpZ25hdHVyZQ"

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+forged)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("算法混淆令牌应被拒绝，实际 %d", w.Code)
	}
}

// 空 Bearer（只有前缀）应被拒绝。
func TestJWTAuth_EmptyBearer(t *testing.T) {
	r := newTestRouter()
	r.GET("/x", JWTAuth("secret"), func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer ")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("空 Bearer 应被拒绝，实际 %d", w.Code)
	}
}

// 格式完全错误的字符串应被拒绝。
func TestJWTAuth_MalformedToken(t *testing.T) {
	r := newTestRouter()
	r.GET("/x", JWTAuth("secret"), func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer not.a.jwt")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("格式错误应被拒绝，实际 %d", w.Code)
	}
}

// claims 缺少 user_id 时视为非法令牌，应拒绝（不能放行为 user_id=0）。
func TestJWTAuth_MissingUserIDClaim(t *testing.T) {
	r := newTestRouter()
	r.GET("/x", JWTAuth("secret"), func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	// 只带 username，不带 user_id
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"username": "ghost",
		"exp":      time.Now().Add(time.Hour).Unix(),
	})
	signed, err := token.SignedString([]byte("secret"))
	if err != nil {
		t.Fatalf("签发令牌失败: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+signed)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("缺少 user_id 的令牌应被拒绝，实际 %d, body=%s", w.Code, w.Body.String())
	}
}

// user_id 类型错误（字符串）时解析失败，应拒绝而非当作 0 放行。
func TestJWTAuth_WrongClaimType(t *testing.T) {
	r := newTestRouter()
	r.GET("/x", JWTAuth("secret"), func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id":  "not-a-number",
		"username": "admin",
		"exp":      time.Now().Add(time.Hour).Unix(),
	})
	signed, err := token.SignedString([]byte("secret"))
	if err != nil {
		t.Fatalf("签发令牌失败: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+signed)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("user_id 类型错误的令牌应被拒绝，实际 %d, body=%s", w.Code, w.Body.String())
	}
}
