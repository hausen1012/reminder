package handlers

import (
	"github.com/gin-gonic/gin"
	"github.com/reminder/backend/internal/middleware"
	"github.com/reminder/backend/internal/services"
)

// AboutHandler 提供关于页的版本信息与更新检查接口。
type AboutHandler struct {
	Svc *services.AboutService
}

// Get GET /api/about
func (h *AboutHandler) Get(c *gin.Context) {
	successJSON(c, h.Svc.Info())
}

// UpdateCheck PUT /api/about/update-check
//
// 只切换自动检查开关；请求体为 {"enabled": true/false}。
func (h *AboutHandler) UpdateCheck(c *gin.Context) {
	var in struct {
		Enabled *bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		abortBindErr(c, err, "请求体格式错误")
		return
	}
	if in.Enabled == nil {
		abortErr(c, middleware.NewAppError(middleware.CodeValidationFailed, "缺少 enabled 字段").WithField("enabled"))
		return
	}
	if err := h.Svc.SetEnabled(*in.Enabled); err != nil {
		abortErr(c, err)
		return
	}
	successJSON(c, h.Svc.Info())
}

// Check POST /api/about/check
//
// 立即检查一次更新，等待结果后返回完整状态。
func (h *AboutHandler) Check(c *gin.Context) {
	h.Svc.CheckNow(c.Request.Context())
	successJSON(c, h.Svc.Info())
}
