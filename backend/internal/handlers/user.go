package handlers

import (
	"github.com/gin-gonic/gin"
	"github.com/reminder/backend/internal/logger"
	"github.com/reminder/backend/internal/middleware"
	"github.com/reminder/backend/internal/services"
)

// UserHandler 处理与当前用户相关的接口。
type UserHandler struct {
	Svc *services.UserService
}

// PasswordRequest 修改密码入参。
type PasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required,min=6"`
}

// UpdatePassword PUT /api/auth/password
func (h *UserHandler) UpdatePassword(c *gin.Context) {
	var req PasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		abortBindErr(c, err, "密码至少 6 个字符")
		return
	}

	userID, ok := currentUserID(c)
	if !ok {
		abortErr(c, middleware.NewAppError(middleware.CodeUnauthorized, "未授权"))
		return
	}

	if err := h.Svc.ChangePassword(userID, req.OldPassword, req.NewPassword); err != nil {
		logger.FromContext(c.Request.Context()).Warn("修改密码失败", "user_id", userID, "error", err)
		abortErr(c, err)
		return
	}
	successJSON(c, nil)
}
