package handlers

import (
	"math"
	"strconv"
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
	// Limiter 可选：为空表示不做登录失败限流（测试与未配置时为 nil）
	Limiter *middleware.LoginLimiter
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

	// 解析出用户名后建立限流计数键
	var limiterKey string
	if h.Limiter != nil {
		limiterKey = middleware.LoginKey(c, req.Username)
		if allowed, retryAfter := h.Limiter.AllowWithRetryAfter(limiterKey); !allowed {
			logger.FromContext(c.Request.Context()).Warn("登录被限流",
				"path", c.Request.URL.Path, "username", req.Username,
				"ip", c.ClientIP(), "retry_after", retryAfter)
			// 按锁定的剩余时长设置 Retry-After
			c.Header("Retry-After", strconv.Itoa(RetryAfterSeconds(retryAfter)))
			abortErr(c, middleware.NewAppError(middleware.CodeRateLimited, "登录失败次数过多，请稍后再试"))
			return
		}
	}

	user, err := h.UserSvc.Authenticate(req.Username, req.Password)
	if err != nil {
		if h.Limiter != nil {
			h.Limiter.RecordFailure(limiterKey)
		}
		// 失败原因（用户不存在 / 密码错）对外统一为同一句，但日志要分开记：
		// 只有「登录失败」三个字，事后无法判断是撞库还是用户记错密码。
		logger.FromContext(c.Request.Context()).Warn("登录失败",
			"path", c.Request.URL.Path, "username", req.Username,
			"ip", c.ClientIP(), "error", err)
		abortErr(c, err)
		return
	}
	if h.Limiter != nil {
		h.Limiter.RecordSuccess(limiterKey)
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

// RetryAfterSeconds 把锁定剩余时长换算成 Retry-After 秒数（向上取整，至少 1）。
func RetryAfterSeconds(d time.Duration) int {
	if d <= 0 {
		return 1
	}
	secs := int(math.Ceil(d.Seconds()))
	if secs < 1 {
		return 1
	}
	return secs
}

// Me GET /api/auth/me
func (h *AuthHandler) Me(c *gin.Context) {
	userID, ok := currentUserID(c)
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

// currentUserID 从 gin.Context 中取 JWT 中间件注入的用户 ID。
func currentUserID(c *gin.Context) (uint, bool) {
	v, exists := c.Get("user_id")
	if !exists {
		return 0, false
	}
	id, ok := v.(uint)
	if !ok || id == 0 {
		return 0, false
	}
	return id, true
}
