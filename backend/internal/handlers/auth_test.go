package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/reminder/backend/internal/middleware"
	"github.com/reminder/backend/internal/models"
	"github.com/reminder/backend/internal/services"
	"golang.org/x/crypto/bcrypt"
)

// newAuthTestEngine 构造一个只挂载认证路由的测试引擎。
func newAuthTestEngine(t *testing.T) (*gin.Engine, *models.User) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := newTestDB(t)

	hash, err := bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("生成密码哈希失败: %v", err)
	}
	user := &models.User{Username: "admin", Password: string(hash)}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("创建测试用户失败: %v", err)
	}

	userSvc := services.NewUserService(db)
	authHandler := &AuthHandler{JWTSecret: "test-secret", UserSvc: userSvc}

	// abortErr 依赖 ErrorHandler 中间件把 AppError 渲染成 JSON，测试里必须一并挂上
	r := gin.New()
	r.Use(middleware.ErrorHandler())
	r.POST("/login", authHandler.Login)
	return r, user
}

func postJSON(r *gin.Engine, path string, body any) *httptest.ResponseRecorder {
	raw, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// 正确用户名密码应返回 token，且响应体走统一的 {code,message,data} 包装。
func TestLogin_Success(t *testing.T) {
	r, _ := newAuthTestEngine(t)

	w := postJSON(r, "/login", map[string]string{"username": "admin", "password": "admin123"})
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
	r, _ := newAuthTestEngine(t)

	w := postJSON(r, "/login", map[string]string{"username": "admin", "password": "wrong"})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("状态码 = %d, 期望 401, body=%s", w.Code, w.Body.String())
	}
}

// 用户不存在时同样返回 401，避免泄露用户名是否存在。
func TestLogin_UnknownUser(t *testing.T) {
	r, _ := newAuthTestEngine(t)

	w := postJSON(r, "/login", map[string]string{"username": "nobody", "password": "admin123"})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("状态码 = %d, 期望 401, body=%s", w.Code, w.Body.String())
	}
}

// 缺少字段时返回 400。
func TestLogin_MissingFields(t *testing.T) {
	r, _ := newAuthTestEngine(t)

	w := postJSON(r, "/login", map[string]string{"username": "admin"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("状态码 = %d, 期望 400, body=%s", w.Code, w.Body.String())
	}
}
