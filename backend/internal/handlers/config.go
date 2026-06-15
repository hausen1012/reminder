package handlers

import (
	"github.com/bedrock/backend/internal/middleware"
	"github.com/bedrock/backend/internal/services"
	"github.com/gin-gonic/gin"
)

// ConfigHandler 站点配置接口。
type ConfigHandler struct {
	Svc *services.ConfigService
}

// GetAll GET /api/config
func (h *ConfigHandler) GetAll(c *gin.Context) {
	cfg, err := h.Svc.GetAll()
	if err != nil {
		abortErr(c, err)
		return
	}
	successJSON(c, cfg)
}

// Update PUT /api/config
func (h *ConfigHandler) Update(c *gin.Context) {
	var in map[string]string
	if err := c.ShouldBindJSON(&in); err != nil {
		abortErr(c, middleware.NewAppError(middleware.CodeValidationFailed, "请求体格式错误"))
		return
	}
	if err := h.Svc.UpsertMap(in); err != nil {
		abortErr(c, err)
		return
	}
	// 返回全量
	cfg, err := h.Svc.GetAll()
	if err != nil {
		abortErr(c, err)
		return
	}
	successJSON(c, cfg)
}
