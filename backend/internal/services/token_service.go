// token_service 管理令牌的全生命周期。
//
// 外部程序通过 X-AUTH 鉴权调用 /api/external/v1/*，面板也可以创建/管理令牌。
// 明文令牌仅创建时一次性返回，后续默认只展示前缀；如需重复查看则由
// GetPlaintext 解密后返回。
//
// 明文用 secretbox 加密后入库（与通道敏感字段同一把密钥）：数据库文件泄漏时
// 不应连带泄漏可直接调用的令牌。解密失败时不回退、不猜测，直接提示重新创建。
package services

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/reminder/backend/internal/crypto/secretbox"
	"github.com/reminder/backend/internal/middleware"
	"github.com/reminder/backend/internal/models"
	"gorm.io/gorm"
)

// TokenService 管理令牌。
type TokenService struct {
	DB  *gorm.DB
	Box *secretbox.Box

	mu         sync.Mutex
	lastUsedAt map[uint]time.Time // 节流 TouchLastUsed
}

// NewTokenService 构造服务。
//
// box 用于加密入库的明文；为 nil 时退化为不加密（仅测试用）。
func NewTokenService(db *gorm.DB, box *secretbox.Box) *TokenService {
	return &TokenService{
		DB:         db,
		Box:        box,
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
	stored, err := s.sealPlaintext(plain)
	if err != nil {
		return "", nil, err
	}
	key := &models.Token{
		Name:      name,
		KeyHash:   hash,
		Prefix:    plain[:len(keyPrefix)+8],
		Plaintext: stored,
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
		defer func() {
			if r := recover(); r != nil {
				slog.Error("TouchLastUsed goroutine panic", "recover", r)
			}
		}()
		now := models.TimeNow()
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
		Where("token_id = ? AND created_at > ?", keyID, models.TimeOf(time.Now().Add(-24*time.Hour))).
		Count(&count)
	return count
}

// UpdateDefaultChannels 更新令牌的默认通道绑定。
func (s *TokenService) UpdateDefaultChannels(id uint, channelIDs []uint) error {
	return s.DB.Transaction(func(tx *gorm.DB) error {
		return s.replaceDefaultChannels(tx, id, channelIDs)
	})
}

// GetPlaintext 返回令牌的明文（从库里解密）。
//
// 读取路径保证不返回 500：
//   - 解密成功，返回明文；
//   - 解密失败（通常是换过 ENCRYPTION_KEY），返回 404 + 可操作提示，
//     而不是把库里的密文当明文返回（用户会误以为那是令牌）。
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
	return s.openPlaintext(key.Plaintext)
}

// --- 明文加解密 ---

// sealPlaintext 加密明文供落库；没有配置 Box 时原样返回（测试场景）。
//
// 加密失败时宁可让创建失败，也不静默降级为明文入库：
// 那会在用户不知情的情况下掏空这层保护。这条路径实际不可达
// （只有 crypto/rand 失败才会触发），因此允许它返回 500。
func (s *TokenService) sealPlaintext(plain string) (string, error) {
	if s.Box == nil {
		return plain, nil
	}
	enc, err := s.Box.EncryptString(plain)
	if err != nil {
		slog.Error("令牌明文加密失败", "error", err)
		return "", middleware.NewAppError(middleware.CodeInternalError, "令牌加密失败")
	}
	return enc, nil
}

// openPlaintext 解密库里的明文，保证不返回 500。
//
//  1. 解密成功 → 返回明文；
//  2. 解密失败 → 返回 404 + 提示重新创建（如 ENCRYPTION_KEY 被更换，密文无法还原）。
func (s *TokenService) openPlaintext(stored string) (string, error) {
	// 未配置加密层（仅测试场景）：库里存的就是明文
	if s.Box == nil {
		return stored, nil
	}
	if plain, err := s.Box.DecryptString(stored); err == nil {
		return plain, nil
	}
	slog.Warn("令牌明文解密失败，可能是 ENCRYPTION_KEY 已更换", "hint", "重新创建该令牌即可恢复查看")
	return "", middleware.NewAppError(middleware.CodeNotFound,
		"该令牌的明文无法解密（ENCRYPTION_KEY 可能已更换），请重新创建令牌")
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
