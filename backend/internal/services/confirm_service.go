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

	"github.com/reminder/backend/internal/config"
	"github.com/reminder/backend/internal/models"
	"gorm.io/gorm"
)

// ConfirmTokenTTL 是确认链接的有效期。
//
// 导出成常量：Dispatch 创建令牌、RecoverChains 判断「这条链还有没有意义」
// 用的是同一个值，两边各写一个字面量迟早会漂移。
const ConfirmTokenTTL = 72 * time.Hour

// ConsumeToken 可能返回的哨兵错误。
//
// 导出成哨兵值而不是字符串：HTTP 层需要区分「已确认」与真正的失败，
// 前者应当渲染成幂等的成功页，而不是吓人的错误页。
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
		ExpiresAt:     models.TimeOf(time.Now().Add(ttl)),
	}
	if err := s.DB.Create(ct).Error; err != nil {
		return "", err
	}
	return token, nil
}

// ConsumeToken 消费一个令牌。
//
// 成功时：标记 used_at、更新 delivery_log 的 confirmed 字段，返回该 delivery_log。
// 错误情况：令牌不存在（ErrConfirmTokenInvalid）、已使用（ErrConfirmTokenUsed）、
// 已过期（ErrConfirmTokenExpired）、日志已被清理（ErrConfirmLogMissing）。
//
// 并发语义：多次点击（浏览器预取、用户连点、多设备同时打开）只会有一次真正生效。
// used_at 的写入是带 `used_at IS NULL` 条件的原子 UPDATE，RowsAffected=0 即说明
// 被别的请求抢先消费，此时按「已使用」处理并尽量返回日志，让上层渲染幂等成功页。
// 已使用时不修改任何数据，也不会重复更新 confirmed_at。
func (s *ConfirmService) ConsumeToken(token string) (*models.DeliveryLog, error) {
	var ct models.ConfirmToken
	if err := s.DB.First(&ct, "token = ?", token).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrConfirmTokenInvalid
		}
		return nil, err
	}

	if ct.UsedAt != nil {
		// 重复点击：不报错页，尽量带回日志让上层渲染「已经确认过了」
		var dl models.DeliveryLog
		if err := s.DB.First(&dl, ct.DeliveryLogID).Error; err == nil {
			return &dl, ErrConfirmTokenUsed
		}
		return nil, ErrConfirmTokenUsed
	}
	if time.Now().After(ct.ExpiresAt.Time) {
		return nil, ErrConfirmTokenExpired
	}

	var target models.DeliveryLog
	if err := s.DB.First(&target, ct.DeliveryLogID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrConfirmLogMissing
		}
		return nil, err
	}

	now := models.TimeNow()
	err := s.DB.Transaction(func(tx *gorm.DB) error {
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

// BuildURL 拼接完整的确认 URL。
//
// 域名优先取设置页的「站点地址」（config 表 key=site_url），没有时回退到环境变量
// SITE_URL。用户点开链接的往往是手机，地址必须手机可达；部署后才发现填错的情况很常见，
// 所以这里现读库，改完无需重启。
func (s *ConfirmService) BuildURL(token string) string {
	fallback := ""
	if s.Cfg != nil {
		fallback = s.Cfg.SiteURL
	}
	return siteURL(s.DB, fallback) + "/c/" + token
}
