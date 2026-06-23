// Package services 业务服务层：handler 与 DB/外部资源之间的胶水。
//
// channel_service 负责通道的 CRUD、敏感字段透明加解密、试发。
// 敏感字段约定：Config JSON 内任意以 "_enc" 结尾的字段值在落库前加密、读出后解密。
package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/bedrock/backend/internal/crypto/secretbox"
	"github.com/bedrock/backend/internal/middleware"
	"github.com/bedrock/backend/internal/models"
	"github.com/bedrock/backend/internal/notifier"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// ChannelService 持有 DB + secretbox 实例。
type ChannelService struct {
	DB  *gorm.DB
	Box *secretbox.Box
}

// NewChannelService 构造服务实例。
func NewChannelService(db *gorm.DB, box *secretbox.Box) *ChannelService {
	return &ChannelService{DB: db, Box: box}
}

// ChannelInput 是 Create/Update 的入参。
//
// Config 是前端传入的明文 map，内含 _enc 后缀字段时由本服务加密后再落库。
type ChannelInput struct {
	Name    string         `json:"name"`
	Type    string         `json:"type"`
	Enabled *bool          `json:"enabled,omitempty"`
	Config  map[string]any `json:"config"`
}

// ChannelView 是返回给前端的视图：Config 内 _enc 后缀字段统一脱敏成 "***"，
// 这样前端看不到明文，编辑时若留空表示不修改。
type ChannelView struct {
	ID        uint           `json:"id"`
	Name      string         `json:"name"`
	Type      string         `json:"type"`
	Enabled   bool           `json:"enabled"`
	Config    map[string]any `json:"config"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// ChannelListFilter 是通道列表分页参数。
type ChannelListFilter struct {
	Enabled   *bool
	Search    string
	Limit     int
	Offset    int
	SortBy    string // created_at | id | name
	SortOrder string // asc | desc
}

const encSuffix = "_enc"

// 占位符常量：前端编辑时未改动敏感字段就回传这个值，服务端识别后不更新。
const sensitivePlaceholder = "***"

func validType(t string) bool {
	switch t {
	case "smtp", "dingtalk", "wecom", "webhook", "log":
		return true
	}
	return false
}

// Create 新建通道。Type 一旦创建不可变更。
func (s *ChannelService) Create(in ChannelInput) (*ChannelView, error) {
	if err := s.validateInput(in, false, nil); err != nil {
		return nil, err
	}
	encConfig, err := s.encryptConfig(in.Config, nil)
	if err != nil {
		return nil, middleware.NewAppError(middleware.CodeValidationFailed, err.Error())
	}
	raw, _ := json.Marshal(encConfig)
	ch := &models.Channel{
		Name:    strings.TrimSpace(in.Name),
		Type:    in.Type,
		Enabled: true,
		Config:  datatypes.JSON(raw),
	}
	if in.Enabled != nil {
		ch.Enabled = *in.Enabled
	}
	if err := s.DB.Create(ch).Error; err != nil {
		if isUniqueErr(err) {
			return nil, middleware.NewAppError(middleware.CodeValidationFailed, "通道名称已存在").WithField("name")
		}
		return nil, err
	}
	return s.toView(ch), nil
}

// Update 修改通道，禁止修改 Type。
//
// Config 中带 _enc 后缀的字段：值为占位符 "***" 时保留原密文不变，其他值视为新明文。
func (s *ChannelService) Update(id uint, in ChannelInput) (*ChannelView, error) {
	ch, err := s.getOrNotFound(id)
	if err != nil {
		return nil, err
	}
	if in.Type != "" && in.Type != ch.Type {
		return nil, middleware.NewAppError(middleware.CodeValidationFailed, "通道类型创建后不可修改").WithField("type")
	}
	in.Type = ch.Type
	if err := s.validateInput(in, true, ch); err != nil {
		return nil, err
	}

	// 取旧的 Config（密文形态），用于占位符回填
	var oldEnc map[string]any
	if len(ch.Config) > 0 {
		_ = json.Unmarshal(ch.Config, &oldEnc)
	}

	encConfig, err := s.encryptConfig(in.Config, oldEnc)
	if err != nil {
		return nil, middleware.NewAppError(middleware.CodeValidationFailed, err.Error())
	}
	raw, _ := json.Marshal(encConfig)

	updates := map[string]any{
		"name":   strings.TrimSpace(in.Name),
		"config": datatypes.JSON(raw),
	}
	if in.Enabled != nil {
		updates["enabled"] = *in.Enabled
	}
	if err := s.DB.Model(ch).Updates(updates).Error; err != nil {
		if isUniqueErr(err) {
			return nil, middleware.NewAppError(middleware.CodeValidationFailed, "通道名称已存在").WithField("name")
		}
		return nil, err
	}
	return s.Get(id)
}

// Get 返回脱敏视图。
func (s *ChannelService) Get(id uint) (*ChannelView, error) {
	ch, err := s.getOrNotFound(id)
	if err != nil {
		return nil, err
	}
	return s.toView(ch), nil
}

// List 返回全部通道（按 id 升序）。
func (s *ChannelService) List() ([]*ChannelView, error) {
	var rows []models.Channel
	if err := s.DB.Order("id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]*ChannelView, 0, len(rows))
	for i := range rows {
		out = append(out, s.toView(&rows[i]))
	}
	return out, nil
}

// ListPaged 返回通道分页结果（按 id 升序）。
func (s *ChannelService) ListPaged(f ChannelListFilter) ([]*ChannelView, int64, error) {
	if f.Limit <= 0 {
		f.Limit = 50
	}
	if f.Limit > 200 {
		f.Limit = 200
	}
	if f.Offset < 0 {
		f.Offset = 0
	}

	q := s.DB.Model(&models.Channel{})
	if f.Enabled != nil {
		q = q.Where("enabled = ?", *f.Enabled)
	}
	if search := strings.TrimSpace(f.Search); search != "" {
		q = q.Where("name LIKE ?", "%"+search+"%")
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []models.Channel
	orderClause := "created_at DESC"
	if f.SortBy != "" {
		orderClause = f.SortBy
		if f.SortOrder == "asc" {
			orderClause += " ASC"
		} else {
			orderClause += " DESC"
		}
	}
	if err := q.Order(orderClause).Limit(f.Limit).Offset(f.Offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}

	out := make([]*ChannelView, 0, len(rows))
	for i := range rows {
		out = append(out, s.toView(&rows[i]))
	}
	return out, total, nil
}

// Delete 删除通道。
func (s *ChannelService) Delete(id uint) error {
	if _, err := s.getOrNotFound(id); err != nil {
		return err
	}
	return s.DB.Delete(&models.Channel{}, id).Error
}

// BatchDelete 批量删除通知通道。
func (s *ChannelService) BatchDelete(ids []uint) error {
	if len(ids) == 0 {
		return nil
	}
	return s.DB.Transaction(func(tx *gorm.DB) error {
		// 先清理关联表
		if err := tx.Where("channel_id IN ?", ids).Delete(&models.ReminderChannel{}).Error; err != nil {
			return err
		}
		return tx.Delete(&models.Channel{}, ids).Error
	})
}

// Toggle 切换启用状态。
func (s *ChannelService) Toggle(id uint) (*ChannelView, error) {
	ch, err := s.getOrNotFound(id)
	if err != nil {
		return nil, err
	}
	ch.Enabled = !ch.Enabled
	if err := s.DB.Model(ch).Update("enabled", ch.Enabled).Error; err != nil {
		return nil, err
	}
	return s.toView(ch), nil
}

// DryRun 用表单配置试发通知，不写 delivery_log。
func (s *ChannelService) DryRun(ctx context.Context, chType string, config map[string]any) error {
	n, err := notifier.Get(chType)
	if err != nil {
		return middleware.NewAppError(middleware.CodeValidationFailed, err.Error())
	}
	// 配置可能含 _enc 后缀字段，dry-run 时前端传的是明文，直接序列化
	plainConfig, _ := json.Marshal(config)

	subject := "通道试发 - " + chType
	body := "这是来自 reminder2 的通道试发消息。\n类型：{{channel_type}}\n时间：{{now}}"
	vars := map[string]string{
		"channel_type": chType,
		"now":          time.Now().Format("2006-01-02 15:04:05"),
		"title":        subject,
		"content":      body,
	}
	rendered := notifier.Message{
		Subject: notifier.Render(subject, vars),
		Body:    notifier.Render(body, vars),
		Vars:    vars,
	}
	if err := n.Send(ctx, plainConfig, rendered); err != nil {
		log.Printf("[channel-dryrun] 试发失败 type=%s: %v", chType, err)
		return err
	}
	return nil
}

// DecryptedConfig 返回明文 config JSON，供 dispatch 阶段直接喂给 Notifier。
//
// dispatch 不应再回经 ChannelView（视图把敏感字段脱敏掉了）。
func (s *ChannelService) DecryptedConfig(ch *models.Channel) ([]byte, error) {
	dec, err := s.decryptConfig(ch.Config)
	if err != nil {
		return nil, err
	}
	return json.Marshal(dec)
}

// --- stats ---

// ChannelStats 是单个通道的发送统计。
type ChannelStats struct {
	ID          uint    `json:"id"`
	Name        string  `json:"name"`
	Type        string  `json:"type"`
	Total       int64   `json:"total"`
	Success     int64   `json:"success"`
	Failed      int64   `json:"failed"`
	SuccessRate float64 `json:"success_rate"`
}

// Stats 返回指定时间窗口内各通道的发送统计。
//
// 没有发送记录的通道也包含在结果中（total=0）。
func (s *ChannelService) Stats(window time.Duration) ([]*ChannelStats, error) {
	cutoff := time.Now().Add(-window)

	type aggRow struct {
		ChannelID   uint
		ChannelName string
		ChannelType string
		Total       int64
		Success     int64
	}
	var rows []aggRow
	s.DB.Model(&models.DeliveryAttempt{}).
		Select("channel_id, channel_name, channel_type, COUNT(*) AS total, SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END) AS success").
		Where("created_at >= ?", cutoff).
		Group("channel_id").
		Scan(&rows)

	// 按 channel_id 建索引
	aggMap := make(map[uint]*aggRow, len(rows))
	for i := range rows {
		aggMap[rows[i].ChannelID] = &rows[i]
	}

	// 拉所有通道，确保无发送记录的通道也出现在结果中
	var channels []models.Channel
	s.DB.Find(&channels)

	out := make([]*ChannelStats, 0, len(channels))
	for i := range channels {
		ch := &channels[i]
		item := &ChannelStats{
			ID:   ch.ID,
			Name: ch.Name,
			Type: ch.Type,
		}
		if a, ok := aggMap[ch.ID]; ok {
			item.Total = a.Total
			item.Success = a.Success
			item.Failed = a.Total - a.Success
			if a.Total > 0 {
				item.SuccessRate = float64(a.Success) / float64(a.Total) * 100
			}
		}
		out = append(out, item)
	}
	return out, nil
}

// --- internal helpers ---

func (s *ChannelService) getOrNotFound(id uint) (*models.Channel, error) {
	var ch models.Channel
	if err := s.DB.First(&ch, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, middleware.NewAppError(middleware.CodeNotFound, "通道不存在")
		}
		return nil, err
	}
	return &ch, nil
}

func (s *ChannelService) validateInput(in ChannelInput, isUpdate bool, ch *models.Channel) error {
	if strings.TrimSpace(in.Name) == "" {
		return middleware.NewAppError(middleware.CodeValidationFailed, "通道名称必填").WithField("name")
	}
	if len([]rune(in.Name)) > 64 {
		return middleware.NewAppError(middleware.CodeValidationFailed, "通道名称最长 64 字符").WithField("name")
	}
	if !validType(in.Type) {
		return middleware.NewAppError(middleware.CodeValidationFailed, "未知通道类型").WithField("type")
	}
	if in.Config == nil {
		return middleware.NewAppError(middleware.CodeValidationFailed, "config 必填").WithField("config")
	}

	cfg := in.Config
	if isUpdate {
		cfg = mergeConfig(ch, in.Config)
	}
	if err := requireFields(in.Type, cfg); err != nil {
		return err
	}
	return nil
}

func requireFields(typ string, cfg map[string]any) error {
	missing := func(field string) error {
		return middleware.NewAppError(middleware.CodeValidationFailed, "缺少必填字段").WithField(field)
	}
	switch typ {
	case "smtp":
		if asString(cfg["host"]) == "" {
			return missing("config.host")
		}
		if asPositiveInt(cfg["port"]) == 0 {
			return missing("config.port")
		}
		if asString(cfg["from_addr"]) == "" {
			return missing("config.from_addr")
		}
		if len(asStringSlice(cfg["to"])) == 0 {
			return missing("config.to")
		}
	case "dingtalk":
		if asString(cfg["webhook_url"]) == "" {
			return missing("config.webhook_url")
		}
	case "wecom":
		if asString(cfg["webhook_url"]) == "" {
			return missing("config.webhook_url")
		}
	case "webhook":
		if asString(cfg["url"]) == "" {
			return missing("config.url")
		}
	case "log":
		// 日志通道无需配置
	}
	return nil
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

func asPositiveInt(v any) int {
	switch n := v.(type) {
	case int:
		if n > 0 {
			return n
		}
	case int32:
		if n > 0 {
			return int(n)
		}
	case int64:
		if n > 0 {
			return int(n)
		}
	case float64:
		if n > 0 {
			return int(n)
		}
	}
	return 0
}

func asStringSlice(v any) []string {
	switch items := v.(type) {
	case []string:
		out := make([]string, 0, len(items))
		for _, item := range items {
			if s := strings.TrimSpace(item); s != "" {
				out = append(out, s)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(items))
		for _, item := range items {
			s, _ := item.(string)
			if s = strings.TrimSpace(s); s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func mergeConfig(ch *models.Channel, patch map[string]any) map[string]any {
	merged := map[string]any{}
	if ch != nil && len(ch.Config) > 0 {
		_ = json.Unmarshal(ch.Config, &merged)
	}
	for k, v := range patch {
		merged[k] = v
	}
	return merged
}

// encryptConfig 把 cfg 内 _enc 后缀字段的明文值加密。
// 当字段值是 sensitivePlaceholder（"***"）时：
//   - oldEnc 中存在该字段：复制原密文；
//   - 否则视为留空。
func (s *ChannelService) encryptConfig(cfg map[string]any, oldEnc map[string]any) (map[string]any, error) {
	out := make(map[string]any, len(cfg))
	for k, v := range cfg {
		if strings.HasSuffix(k, encSuffix) {
			str, ok := v.(string)
			if !ok {
				return nil, fmt.Errorf("字段 %s 必须是字符串", k)
			}
			if str == sensitivePlaceholder {
				if oldEnc != nil {
					if old, ok := oldEnc[k]; ok {
						out[k] = old
						continue
					}
				}
				out[k] = ""
				continue
			}
			if str == "" {
				out[k] = ""
				continue
			}
			enc, err := s.Box.EncryptString(str)
			if err != nil {
				return nil, err
			}
			out[k] = enc
		} else {
			out[k] = v
		}
	}
	return out, nil
}

// decryptConfig 把 cfg 内 _enc 后缀字段的密文解密为明文。
func (s *ChannelService) decryptConfig(raw datatypes.JSON) (map[string]any, error) {
	cfg := map[string]any{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return nil, err
		}
	}
	for k, v := range cfg {
		if !strings.HasSuffix(k, encSuffix) {
			continue
		}
		str, ok := v.(string)
		if !ok || str == "" {
			continue
		}
		plain, err := s.Box.DecryptString(str)
		if err != nil {
			return nil, fmt.Errorf("字段 %s 解密失败: %w", k, err)
		}
		cfg[k] = plain
	}
	return cfg, nil
}

// toView 把 Channel 转成脱敏视图：_enc 字段统一显示为 "***"。
func (s *ChannelService) toView(ch *models.Channel) *ChannelView {
	cfg := map[string]any{}
	if len(ch.Config) > 0 {
		_ = json.Unmarshal(ch.Config, &cfg)
	}
	for k, v := range cfg {
		if !strings.HasSuffix(k, encSuffix) {
			continue
		}
		str, _ := v.(string)
		if str == "" {
			cfg[k] = ""
		} else {
			cfg[k] = sensitivePlaceholder
		}
	}
	return &ChannelView{
		ID:        ch.ID,
		Name:      ch.Name,
		Type:      ch.Type,
		Enabled:   ch.Enabled,
		Config:    cfg,
		CreatedAt: ch.CreatedAt,
		UpdatedAt: ch.UpdatedAt,
	}
}

// isUniqueErr 粗略识别 SQLite 唯一索引冲突。
func isUniqueErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed") || strings.Contains(msg, "unique constraint")
}
