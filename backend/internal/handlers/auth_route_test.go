package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/reminder/backend/internal/middleware"
	"github.com/reminder/backend/internal/models"
	"github.com/reminder/backend/internal/services"
	"golang.org/x/crypto/bcrypt"
)

const testJWTSecret = "test-secret"

// newAuthRouteTestEngine 构造含 JWT 保护路由的引擎，返回引擎与合法令牌。
func newAuthRouteTestEngine(t *testing.T) (*gin.Engine, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := newTestDB(t)

	// 造一个已知密码的用户
	hash, err := bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("生成密码哈希失败: %v", err)
	}
	user := &models.User{Username: "admin", Password: string(hash)}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("创建用户失败: %v", err)
	}

	userSvc := services.NewUserService(db)
	authHandler := &AuthHandler{JWTSecret: testJWTSecret, UserSvc: userSvc}
	userHandler := &UserHandler{Svc: userSvc}

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.ErrorHandler())

	auth := r.Group("/auth")
	auth.Use(middleware.JWTAuth(testJWTSecret))
	{
		auth.GET("/me", authHandler.Me)
		auth.POST("/logout", authHandler.Logout)
		auth.PUT("/password", userHandler.UpdatePassword)
	}
	return r, signAuthToken(t, user.ID)
}

// signAuthToken 签发一个测试用 JWT。
func signAuthToken(t *testing.T, userID uint) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id":  userID,
		"username": "admin",
		"exp":      time.Now().Add(time.Hour).Unix(),
	})
	s, err := token.SignedString([]byte(testJWTSecret))
	if err != nil {
		t.Fatalf("签发令牌失败: %v", err)
	}
	return s
}

// authed 发起一次带 Bearer 令牌的请求。
func authed(r *gin.Engine, method, path, token string, body any) *httptest.ResponseRecorder {
	req := newRequest(method, path, body)
	req.Header.Set("Authorization", "Bearer "+token)
	return serve(r, req)
}

// Me 应返回当前用户信息。
func TestAuthMe(t *testing.T) {
	r, token := newAuthRouteTestEngine(t)

	w := authed(r, http.MethodGet, "/auth/me", token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, body=%s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			Username string `json:"username"`
		} `json:"data"`
	}
	decodeJSON(t, w, &resp)
	if resp.Data.Username != "admin" {
		t.Fatalf("username = %q", resp.Data.Username)
	}
}

// Logout 应返回成功（JWT 无状态，前端丢弃令牌即可）。
func TestAuthLogout(t *testing.T) {
	r, token := newAuthRouteTestEngine(t)

	if w := authed(r, http.MethodPost, "/auth/logout", token, nil); w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d", w.Code)
	}
}

// 修改密码：原密码错 400，原密码对则成功。
func TestAuthUpdatePassword(t *testing.T) {
	r, token := newAuthRouteTestEngine(t)

	w := authed(r, http.MethodPut, "/auth/password", token, map[string]string{
		"old_password": "wrong",
		"new_password": "newpass123",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("原密码错应 400，实际 %d", w.Code)
	}

	w = authed(r, http.MethodPut, "/auth/password", token, map[string]string{
		"old_password": "admin123",
		"new_password": "newpass123",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("修改密码应成功，实际 %d, body=%s", w.Code, w.Body.String())
	}
}

// 新密码过短应 400（binding min=6）。
func TestAuthUpdatePassword_TooShort(t *testing.T) {
	r, token := newAuthRouteTestEngine(t)

	w := authed(r, http.MethodPut, "/auth/password", token, map[string]string{
		"old_password": "admin123",
		"new_password": "123",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("状态码 = %d, 期望 400", w.Code)
	}
}

// 无令牌访问受保护路由应 401。
func TestAuthProtectedRoutes_RequireToken(t *testing.T) {
	r, _ := newAuthRouteTestEngine(t)

	if w := doJSON(r, http.MethodGet, "/auth/me", nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("/me 应 401，实际 %d", w.Code)
	}
	if w := doJSON(r, http.MethodPut, "/auth/password", map[string]string{"old_password": "a", "new_password": "bbbbbb"}); w.Code != http.StatusUnauthorized {
		t.Fatalf("/password 应 401，实际 %d", w.Code)
	}
}
