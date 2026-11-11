// confirm_service 管理一次性的确认令牌：触发时创建，访问 /c/:token 时标记已确认，
// ConfirmRetryManager 据此终止重发链。
package services

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/reminder/backend/internal/config"
	"github.com/reminder/backend/internal/models"
	"gorm.io/gorm"
)

// ConfirmTokenTTL 是确认链接的有效期。创建令牌与恢复链时共用，避免两处字面量漂移。
const ConfirmTokenTTL = 72 * time.Hour

// ConsumeToken 可能返回的哨兵错误：HTTP 层需要区分「已确认」与真正的失败，
// 前者应渲染成幂等的成功页。
var (
	// ErrConfirmTokenInvalid 令牌不存在。
	ErrConfirmTokenInvalid = errors.New("确认链接无效")
	// ErrConfirmTokenUsed 令牌已被消费（含并发抢先的情况）。
	ErrConfirmTokenUsed = errors.New("该确认链接已被使用")
	// ErrConfirmTokenExpired 令牌已过有效期。
	ErrConfirmTokenExpired = errors.New("确认链接已过期")
	// ErrConfirmLogMissing 令牌对应的投递日志已被清理。
	ErrConfirmLogMissing = errors.New("确认记录不存在")
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

// CreateToken 生成一个 32 字节 hex 令牌并落库，ttl 为有效期。
func (s *ConfirmService) CreateToken(deliveryLogID uint, ttl time.Duration) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := hex.EncodeToString(b)

	ct := &models.ConfirmToken{
		Token:         token,
		DeliveryLogID: deliveryLogID,
		ExpiresAt:     models.TimeOf(time.Now().Add(ttl)),
	}
	if err := s.DB.Create(ct).Error; err != nil {
		return "", err
	}
	return token, nil
}

// InspectToken 查询令牌状态并返回对应的投递日志（不消费令牌）。
//
// 令牌已使用时返回 ErrConfirmTokenUsed 与日志（便于渲染已确认页）；
// 令牌不存在返回 ErrConfirmTokenInvalid；已过期返回 ErrConfirmTokenExpired；
// 对应日志已被清理返回 ErrConfirmLogMissing。
func (s *ConfirmService) InspectToken(token string) (*models.DeliveryLog, error) {
	ct, dl, err := s.loadToken(token)
	if err != nil {
		return nil, err
	}
	if ct.UsedAt != nil {
		return dl, ErrConfirmTokenUsed
	}
	if time.Now().After(ct.ExpiresAt.Time) {
		return nil, ErrConfirmTokenExpired
	}
	return dl, nil
}

// ConsumeToken 消费一个令牌，成功时标记 used_at 并把该链的日志标为已确认。
//
// 并发点击只有一次真正生效：used_at 用带 `used_at IS NULL` 条件的原子 UPDATE 写入，
// RowsAffected=0 即说明被抢先，按「已使用」处理并尽量返回日志，以便渲染幂等成功页。
func (s *ConfirmService) ConsumeToken(token string) (*models.DeliveryLog, error) {
	ct, target, err := s.loadToken(token)
	if err != nil {
		return nil, err
	}

	if ct.UsedAt != nil {
		// 重复点击：不报错页，尽量带回日志让上层渲染「已经确认过了」
		return target, ErrConfirmTokenUsed
	}
	if time.Now().After(ct.ExpiresAt.Time) {
		return nil, ErrConfirmTokenExpired
	}

	now := models.TimeNow()
	err = s.DB.Transaction(func(tx *gorm.DB) error {
		// 条件更新保证只有一个请求能把 used_at 从 NULL 写成 now
		res := tx.Model(&models.ConfirmToken{}).
			Where("token = ? AND used_at IS NULL", token).
			Update("used_at", now)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrConfirmTokenUsed
		}
		updates := map[string]any{
			"confirmed":    true,
			"confirmed_at": now,
		}
		if target.ConfirmChainID != nil && *target.ConfirmChainID != "" {
			return tx.Model(&models.DeliveryLog{}).
				Where("confirm_chain_id = ?", *target.ConfirmChainID).
				Updates(updates).Error
		}
		return tx.Model(&models.DeliveryLog{}).
			Where("id = ?", ct.DeliveryLogID).
			Updates(updates).Error
	})
	if err != nil {
		if errors.Is(err, ErrConfirmTokenUsed) {
			// 并发被抢先：数据已被对方写好，直接读回交给上层
			var dl models.DeliveryLog
			if e := s.DB.First(&dl, ct.DeliveryLogID).Error; e == nil {
				return &dl, ErrConfirmTokenUsed
			}
		}
		return nil, err
	}

	var dl models.DeliveryLog
	if err := s.DB.First(&dl, ct.DeliveryLogID).Error; err != nil {
		return nil, err
	}
	return &dl, nil
}

// BuildURL 拼接完整的确认 URL，域名取设置页的站点地址（现读库，改完即时生效）。
func (s *ConfirmService) BuildURL(token string) string {
	fallback := ""
	if s.Cfg != nil {
		fallback = s.Cfg.SiteURL
	}
	return siteURL(s.DB, fallback) + "/c/" + token
}

// loadToken 读取令牌及其对应的投递日志。
//
// 令牌不存在返回 ErrConfirmTokenInvalid；日志已被清理返回 ErrConfirmLogMissing
// （此时令牌本身仍可用作「已使用」的判定依据，因此一并返回）。
func (s *ConfirmService) loadToken(token string) (*models.ConfirmToken, *models.DeliveryLog, error) {
	var ct models.ConfirmToken
	if err := s.DB.First(&ct, "token = ?", token).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrConfirmTokenInvalid
		}
		return nil, nil, err
	}

	var dl models.DeliveryLog
	if err := s.DB.First(&dl, ct.DeliveryLogID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &ct, nil, ErrConfirmLogMissing
		}
		return nil, nil, err
	}
	return &ct, &dl, nil
}
