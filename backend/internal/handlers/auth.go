package handlers

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/reminder/backend/internal/logger"
	"github.com/reminder/backend/internal/middleware"
	"github.com/reminder/backend/internal/services"
)

// AuthHandler 处理登录、当前用户与退出。
type AuthHandler struct {
	JWTSecret string
	UserSvc   *services.UserService
}

// LoginRequest 登录入参。
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// Login POST /api/auth/login
func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		abortBindErr(c, err, "请输入用户名和密码")
		return
	}

	user, err := h.UserSvc.Authenticate(req.Username, req.Password)
	if err != nil {
		// 失败原因（用户不存在 / 密码错）对外统一为同一句，但日志要分开记：
		// 只有「登录失败」三个字，事后无法判断是撞库还是用户记错密码。
		logger.FromContext(c.Request.Context()).Warn("登录失败",
			"path", c.Request.URL.Path, "username", req.Username,
			"ip", c.ClientIP(), "error", err)
		abortErr(c, err)
		return
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id":  user.ID,
		"username": user.Username,
		"exp":      time.Now().Add(72 * time.Hour).Unix(),
	})
	tokenStr, err := token.SignedString([]byte(h.JWTSecret))
	if err != nil {
		logger.FromContext(c.Request.Context()).Error("签发 JWT 失败",
			"path", c.Request.URL.Path, "user_id", user.ID, "error", err)
		abortErr(c, err)
		return
	}
	logger.FromContext(c.Request.Context()).Info("登录成功",
		"user_id", user.ID, "username", user.Username)

	successJSON(c, gin.H{
		"token": tokenStr,
		"user": gin.H{
			"id":       user.ID,
			"username": user.Username,
		},
	})
}

// Me GET /api/auth/me
func (h *AuthHandler) Me(c *gin.Context) {
	userID, ok := middleware.UserIDOf(c)
	if !ok {
		abortErr(c, middleware.NewAppError(middleware.CodeUnauthorized, "未授权"))
		return
	}
	user, err := h.UserSvc.Get(userID)
	if err != nil {
		abortErr(c, err)
		return
	}
	successJSON(c, gin.H{
		"id":         user.ID,
		"username":   user.Username,
		"created_at": user.CreatedAt,
	})
}

// Logout POST /api/auth/logout
func (h *AuthHandler) Logout(c *gin.Context) {
	successJSON(c, nil)
}
