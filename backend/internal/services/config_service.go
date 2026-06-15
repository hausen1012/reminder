package services

import (
	"github.com/bedrock/backend/internal/models"
	"gorm.io/gorm"
)

// ConfigService 管理站点配置。
type ConfigService struct {
	DB *gorm.DB
}

// NewConfigService 构造服务。
func NewConfigService(db *gorm.DB) *ConfigService {
	return &ConfigService{DB: db}
}

// GetAll 读取全部配置，返回 key→value 映射。
func (s *ConfigService) GetAll() (map[string]string, error) {
	var rows []models.Config
	if err := s.DB.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[string]string, len(rows))
	for _, r := range rows {
		out[r.Key] = r.Value
	}
	return out, nil
}

// UpsertMap 批量更新配置。
// value 为空字符串时删除对应 key。
func (s *ConfigService) UpsertMap(cfg map[string]string) error {
	for k, v := range cfg {
		if v == "" {
			if err := s.DB.Delete(&models.Config{}, "key = ?", k).Error; err != nil {
				return err
			}
			continue
		}
		// GORM Upsert 风格：Clauses(clause.OnConflict{...})
		if err := s.DB.
			Where("key = ?", k).
			Assign(models.Config{Value: v}).
			FirstOrCreate(&models.Config{Key: k, Value: v}).Error; err != nil {
			return err
		}
	}
	return nil
}
