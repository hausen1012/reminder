package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

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
