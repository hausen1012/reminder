package handlers

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/reminder/backend/internal/middleware"
	"github.com/reminder/backend/internal/services"
)

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
	w := postJSON(r, "/login", map[string]string{"username": "admin", "password": "admin123"})
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
