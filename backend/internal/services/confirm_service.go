// confirm_service 管理一次性的确认令牌，用于"需要确认"的提醒场景。
//
// 一条提醒触发时创建 token，用户访问 /c/:token 时标记为已确认；
// ConfirmRetryManager 在重发前检查是否已确认，已确认则终止重发链。
package services

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/bedrock/backend/internal/config"
	"github.com/bedrock/backend/internal/models"
	"gorm.io/gorm"
)

// ConfirmService 提供令牌的创建、消费与 URL 拼接。
type ConfirmService struct {
	DB  *gorm.DB
	Cfg *config.Config
}

// NewConfirmService 构造确认服务。
func NewConfirmService(db *gorm.DB, cfg *config.Config) *ConfirmService {
	return &ConfirmService{DB: db, Cfg: cfg}
}

// CreateToken 生成一个 32 字节 hex 令牌并落库。
//
// deliveryLogID 是第一次触发的日志 ID；ttl 是令牌有效期，
// 超过有效期后 ConsumeToken 返回"已过期"。
func (s *ConfirmService) CreateToken(deliveryLogID uint, ttl time.Duration) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := hex.EncodeToString(b)

	ct := &models.ConfirmToken{
		Token:         token,
		DeliveryLogID: deliveryLogID,
		ExpiresAt:     time.Now().Add(ttl),
	}
	if err := s.DB.Create(ct).Error; err != nil {
		return "", err
	}
	return token, nil
}

// ConsumeToken 消费一个令牌。
//
// 成功时：标记 used_at、更新 delivery_log 的 confirmed 字段，
// 返回该 delivery_log。
// 错误情况：令牌不存在、已使用、已过期。
func (s *ConfirmService) ConsumeToken(token string) (*models.DeliveryLog, error) {
	var ct models.ConfirmToken
	if err := s.DB.First(&ct, "token = ?", token).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("确认链接无效")
		}
		return nil, err
	}
	if ct.UsedAt != nil {
		return nil, errors.New("该确认链接已被使用")
	}
	if time.Now().After(ct.ExpiresAt) {
		return nil, errors.New("确认链接已过期")
	}

	var target models.DeliveryLog
	if err := s.DB.First(&target, ct.DeliveryLogID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("确认记录不存在")
		}
		return nil, err
	}

	now := time.Now()
	err := s.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&ct).Update("used_at", now).Error; err != nil {
			return err
		}
		updates := map[string]any{
			"confirmed":    true,
			"confirmed_at": now,
		}
		if target.ConfirmChainID != nil && *target.ConfirmChainID != "" {
			if err := tx.Model(&models.DeliveryLog{}).
				Where("confirm_chain_id = ?", *target.ConfirmChainID).
				Updates(updates).Error; err != nil {
				return err
			}
			return nil
		}
		if err := tx.Model(&models.DeliveryLog{}).
			Where("id = ?", ct.DeliveryLogID).
			Updates(updates).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	var dl models.DeliveryLog
	if err := s.DB.First(&dl, ct.DeliveryLogID).Error; err != nil {
		return nil, err
	}
	return &dl, nil
}

// BuildURL 用 PublicBaseURL 拼接完整的确认 URL。
func (s *ConfirmService) BuildURL(token string) string {
	base := s.Cfg.PublicBaseURL
	if base == "" {
		base = "http://localhost:8080"
	}
	return base + "/c/" + token
}
