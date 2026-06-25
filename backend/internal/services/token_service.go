// token_service 管理令牌的全生命周期。
//
// 外部程序通过 X-AUTH 鉴权调用 /api/external/v1/*，面板也可以创建/管理令牌。
// 明文令牌仅创建时一次性返回，后续默认只展示前缀；如需重复查看则直接读取明文。
package services

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/reminder/backend/internal/middleware"
	"github.com/reminder/backend/internal/models"
	"gorm.io/gorm"
)

// TokenService 管理令牌。
type TokenService struct {
	DB *gorm.DB

	mu         sync.Mutex
	lastUsedAt map[uint]time.Time // 节流 TouchLastUsed
}

// NewTokenService 构造服务。
func NewTokenService(db *gorm.DB) *TokenService {
	return &TokenService{
		DB:         db,
		lastUsedAt: make(map[uint]time.Time),
	}
}

const keyPrefix = "bdrk_"
const keyBytes = 24

// Create 生成令牌，返回明文与模型。
func (s *TokenService) Create(name string, defaultChannelIDs []uint) (string, *models.Token, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", nil, middleware.NewAppError(middleware.CodeValidationFailed, "名称必填").WithField("name")
	}

	plain, err := generateKey()
	if err != nil {
		return "", nil, err
	}

	hash := sha256Hex(plain)
	key := &models.Token{
		Name:      name,
		KeyHash:   hash,
		Prefix:    plain[:len(keyPrefix)+8],
		Plaintext: plain,
		Enabled:   true,
	}

	err = s.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(key).Error; err != nil {
			return err
		}
		return s.replaceDefaultChannels(tx, key.ID, defaultChannelIDs)
	})
	if err != nil {
		return "", nil, err
	}
	return plain, key, nil
}

// List 返回所有令牌，支持分页和搜索。
func (s *TokenService) List(limit, offset int, search string) ([]*models.Token, int64, error) {
	var rows []models.Token
	query := s.DB.Model(&models.Token{})
	if search != "" {
		query = query.Where("name LIKE ?", "%"+search+"%")
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := query.Order("id DESC").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	out := make([]*models.Token, len(rows))
	for i := range rows {
		out[i] = &rows[i]
	}
	return out, total, nil
}

// Toggle 启用/禁用。
func (s *TokenService) Toggle(id uint) (*models.Token, error) {
	var key models.Token
	if err := s.DB.First(&key, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, middleware.NewAppError(middleware.CodeNotFound, "令牌不存在")
		}
		return nil, err
	}
	key.Enabled = !key.Enabled
	if err := s.DB.Model(&key).Update("enabled", key.Enabled).Error; err != nil {
		return nil, err
	}
	return &key, nil
}

// Delete 删除令牌。
func (s *TokenService) Delete(id uint) error {
	var key models.Token
	if err := s.DB.First(&key, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return middleware.NewAppError(middleware.CodeNotFound, "令牌不存在")
		}
		return err
	}
	return s.DB.Delete(&key).Error
}

// Verify 验证明文令牌是否有效。
func (s *TokenService) Verify(plain string) (*models.Token, bool) {
	if !strings.HasPrefix(plain, keyPrefix) {
		return nil, false
	}
	hash := sha256Hex(plain)
	var key models.Token
	if err := s.DB.Where("key_hash = ?", hash).First(&key).Error; err != nil {
		return nil, false
	}
	if !key.Enabled {
		return nil, false
	}
	return &key, true
}

// TouchLastUsed 更新 LastUsedAt（节流：每分钟/令牌最多一次）。
func (s *TokenService) TouchLastUsed(keyID uint) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if last, ok := s.lastUsedAt[keyID]; ok && time.Since(last) < time.Minute {
		return
	}
	s.lastUsedAt[keyID] = time.Now()
	go func() {
		now := time.Now()
		_ = s.DB.Model(&models.Token{}).Where("id = ?", keyID).Update("last_used_at", now).Error
	}()
}

// DefaultChannelIDs 返回令牌绑定的默认通道 ID 列表。
func (s *TokenService) DefaultChannelIDs(keyID uint) []uint {
	var ids []uint
	s.DB.Model(&models.TokenDefaultChannel{}).
		Where("token_id = ?", keyID).
		Pluck("channel_id", &ids)
	return ids
}

// Stats24h 返回近 24 小时使用次数（按 reminders 表 TokenID 统计）。
func (s *TokenService) Stats24h(keyID uint) int64 {
	var count int64
	s.DB.Model(&models.Reminder{}).
		Where("token_id = ? AND created_at > ?", keyID, time.Now().Add(-24*time.Hour)).
		Count(&count)
	return count
}

// UpdateDefaultChannels 更新令牌的默认通道绑定。
func (s *TokenService) UpdateDefaultChannels(id uint, channelIDs []uint) error {
	return s.DB.Transaction(func(tx *gorm.DB) error {
		return s.replaceDefaultChannels(tx, id, channelIDs)
	})
}

// GetPlaintext 返回令牌的明文。
func (s *TokenService) GetPlaintext(id uint) (string, error) {
	var key models.Token
	if err := s.DB.First(&key, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", middleware.NewAppError(middleware.CodeNotFound, "令牌不存在")
		}
		return "", err
	}
	if strings.TrimSpace(key.Plaintext) == "" {
		return "", middleware.NewAppError(middleware.CodeNotFound, "该令牌暂无可查看的明文，请重新创建")
	}
	return key.Plaintext, nil
}

// --- internal ---

func (s *TokenService) replaceDefaultChannels(tx *gorm.DB, keyID uint, ids []uint) error {
	if err := tx.Where("token_id = ?", keyID).Delete(&models.TokenDefaultChannel{}).Error; err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	rows := make([]models.TokenDefaultChannel, 0, len(ids))
	seen := map[uint]bool{}
	for _, id := range ids {
		if id == 0 || seen[id] {
			continue
		}
		seen[id] = true
		rows = append(rows, models.TokenDefaultChannel{TokenID: keyID, ChannelID: id})
	}
	return tx.Create(&rows).Error
}

// generateKey 生成 bdrk_ + 24 字节 base62 令牌。
func generateKey() (string, error) {
	b := make([]byte, keyBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return keyPrefix + base62Encode(b), nil
}

// sha256Hex 对明文做 sha256 返回 hex。
func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// base62Encode 把字节编码为 base62 字符串。
func base62Encode(b []byte) string {
	const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	n := new(big.Int).SetBytes(b)
	out := make([]byte, 0, 32)
	zero := big.NewInt(0)
	base := big.NewInt(62)
	mod := new(big.Int)
	for n.Cmp(zero) > 0 {
		n.DivMod(n, base, mod)
		out = append(out, chars[mod.Int64()])
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	if len(out) == 0 {
		return string(chars[0])
	}
	return string(out)
}

// TokenView 是前端的富视图。
type TokenView struct {
	models.Token
	DefaultChannelIDs []uint `json:"default_channel_ids"`
	Usage24h          int64  `json:"usage_24h"`
}

// ListViews 返回令牌富列表，支持分页和搜索。
func (s *TokenService) ListViews(limit, offset int, search string) ([]*TokenView, int64, error) {
	keys, total, err := s.List(limit, offset, search)
	if err != nil {
		return nil, 0, err
	}
	views := make([]*TokenView, 0, len(keys))
	for _, k := range keys {
		views = append(views, &TokenView{
			Token:             *k,
			DefaultChannelIDs: s.DefaultChannelIDs(k.ID),
			Usage24h:          s.Stats24h(k.ID),
		})
	}
	return views, total, nil
}

// GetView 返回单个令牌的富视图。
func (s *TokenService) GetView(id uint) (*TokenView, error) {
	var key models.Token
	if err := s.DB.First(&key, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, middleware.NewAppError(middleware.CodeNotFound, "令牌不存在")
		}
		return nil, err
	}
	return &TokenView{
		Token:             key,
		DefaultChannelIDs: s.DefaultChannelIDs(key.ID),
		Usage24h:          s.Stats24h(key.ID),
	}, nil
}

var _ = fmt.Sprintf