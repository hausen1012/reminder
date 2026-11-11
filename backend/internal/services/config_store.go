// config_store 是 config 表（key-value 站点配置）的统一读写封装。
//
// 读取按 key 或前缀（如 ai_）返回 map；写入时空值表示删除该 key。
package services

import (
	"errors"

	"github.com/reminder/backend/internal/models"
	"gorm.io/gorm"
)

// configStore 封装 config 表的最小读写能力。
type configStore struct {
	db *gorm.DB
}

// get 读取单个配置项；不存在时返回 ok=false 而不是错误。
func (s configStore) get(key string) (value string, ok bool, err error) {
	var row models.Config
	if err := s.db.First(&row, "key = ?", key).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", false, nil
		}
		return "", false, err
	}
	return row.Value, true, nil
}

// set 写入单个配置项；空值表示删除该 key。
func (s configStore) set(key, value string) error {
	if value == "" {
		return s.delete(key)
	}
	return s.db.Where("key = ?", key).
		Assign(models.Config{Value: value}).
		FirstOrCreate(&models.Config{Key: key, Value: value}).Error
}

// delete 删除单个配置项。
func (s configStore) delete(key string) error {
	return s.db.Delete(&models.Config{}, "key = ?", key).Error
}

// all 返回全部配置。
func (s configStore) all() (map[string]string, error) {
	return s.allWhere(s.db.Model(&models.Config{}))
}

// allWithPrefix 返回指定前缀的配置（前缀按字面量匹配，不把 "_" 当通配符）。
func (s configStore) allWithPrefix(prefix string) (map[string]string, error) {
	if prefix == "" {
		return s.all()
	}
	q := s.db.Model(&models.Config{}).Where("substr(key, 1, ?) = ?", len(prefix), prefix)
	return s.allWhere(q)
}

// bool 读取布尔配置：未落库时返回 def，值为 "false" 时返回 false。
func (s configStore) bool(key string, def bool) (bool, error) {
	v, ok, err := s.get(key)
	if err != nil || !ok {
		return def, err
	}
	return v != "false", nil
}

// allWhere 执行查询并转成 map。
func (s configStore) allWhere(q *gorm.DB) (map[string]string, error) {
	var rows []models.Config
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[string]string, len(rows))
	for _, r := range rows {
		out[r.Key] = r.Value
	}
	return out, nil
}
