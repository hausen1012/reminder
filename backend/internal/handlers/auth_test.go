// 认证相关用例：登录、令牌签发与校验、受保护路由、改密。
//
// 合并自 auth_test.go / auth_route_test.go / login_flow_test.go。
package handlers

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/reminder/backend/internal/middleware"
	"github.com/reminder/backend/internal/services"
)

const testJWTSecret = "test-secret"

// newAuthTestEngine 构造一个只挂载登录路由的测试引擎。
func newAuthTestEngine(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := newTestDB(t)
	seedUser(t, db)

	userSvc := services.NewUserService(db)
	authHandler := &AuthHandler{JWTSecret: testJWTSecret, UserSvc: userSvc}

	// abortErr 依赖 ErrorHandler 中间件把 AppError 渲染成 JSON，测试里必须一并挂上
	r := gin.New()
	r.Use(middleware.ErrorHandler())
	r.POST("/login", authHandler.Login)
	return r
}

// newAuthRouteTestEngine 构造含 JWT 保护路由的引擎，返回引擎与合法令牌。
func newAuthRouteTestEngine(t *testing.T) (*gin.Engine, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := newTestDB(t)
	user := seedUser(t, db)

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

// 正确用户名密码应返回 token，且响应体走统一的 {code,message,data} 包装。
func TestLogin_Success(t *testing.T) {
	r := newAuthTestEngine(t)

	w := doJSON(r, http.MethodPost, "/login", map[string]string{"username": "admin", "password": "admin123"})
	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, 期望 200, body=%s", w.Code, w.Body.String())
	}

	var resp struct {
		Code int `json:"code"`
		Data struct {
			Token string `json:"token"`
			User  struct {
				Username string `json:"username"`
			} `json:"user"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("响应不是合法 JSON: %v", err)
	}
	if resp.Data.Token == "" {
		t.Fatal("期望返回 token")
	}
	if resp.Data.User.Username != "admin" {
		t.Fatalf("username = %q, 期望 admin", resp.Data.User.Username)
	}
}

// 密码错误必须是 401，且错误码统一为 unauthorized。
func TestLogin_WrongPassword(t *testing.T) {
	r := newAuthTestEngine(t)

	w := doJSON(r, http.MethodPost, "/login", map[string]string{"username": "admin", "password": "wrong"})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("状态码 = %d, 期望 401, body=%s", w.Code, w.Body.String())
	}
}

// 用户不存在时同样返回 401，避免泄露用户名是否存在。
func TestLogin_UnknownUser(t *testing.T) {
	r := newAuthTestEngine(t)

	w := doJSON(r, http.MethodPost, "/login", map[string]string{"username": "nobody", "password": "admin123"})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("状态码 = %d, 期望 401, body=%s", w.Code, w.Body.String())
	}
}

// 缺少字段时返回 400。
func TestLogin_MissingFields(t *testing.T) {
	r := newAuthTestEngine(t)

	w := doJSON(r, http.MethodPost, "/login", map[string]string{"username": "admin"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("状态码 = %d, 期望 400, body=%s", w.Code, w.Body.String())
	}
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

// 端到端：登录签发的令牌必须能被 JWTAuth 接受。
//
// 这条用例防的是"签发端用 MapClaims、校验端用结构化 claims"时
// user_id 类型不匹配导致所有已登录用户被拒的回归。
func TestLoginTokenAcceptedByJWTAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newTestDB(t)
	seedUser(t, db)

	userSvc := services.NewUserService(db)
	authHandler := &AuthHandler{JWTSecret: testJWTSecret, UserSvc: userSvc}

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.ErrorHandler())
	r.POST("/login", authHandler.Login)

	protected := r.Group("/protected")
	protected.Use(middleware.JWTAuth(testJWTSecret))
	protected.GET("/me", authHandler.Me)

	// 登录拿令牌
	w := doJSON(r, http.MethodPost, "/login", map[string]string{"username": "admin", "password": "admin123"})
	if w.Code != http.StatusOK {
		t.Fatalf("登录失败: %d, body=%s", w.Code, w.Body.String())
	}
	var loginResp struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	decodeJSON(t, w, &loginResp)
	if loginResp.Data.Token == "" {
		t.Fatal("登录未返回 token")
	}

	// 用该令牌访问受保护接口
	req := newRequest(http.MethodGet, "/protected/me", nil)
	req.Header.Set("Authorization", "Bearer "+loginResp.Data.Token)
	w = serve(r, req)
	if w.Code != http.StatusOK {
		t.Fatalf("登录令牌应可访问受保护接口，实际 %d, body=%s", w.Code, w.Body.String())
	}

	var meResp struct {
		Data struct {
			Username string `json:"username"`
		} `json:"data"`
	}
	decodeJSON(t, w, &meResp)
	if meResp.Data.Username != "admin" {
		t.Fatalf("username = %q", meResp.Data.Username)
	}
}
