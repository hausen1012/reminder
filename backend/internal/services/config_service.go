package services

import (
	"log/slog"

	"github.com/reminder/backend/internal/middleware"
	"github.com/reminder/backend/internal/models"
	"github.com/reminder/backend/internal/timezone"
	"gorm.io/gorm"
)

// KeyTimezone 是站点时区在 config 表中的 key。
const KeyTimezone = "timezone"

// ConfigService 管理站点配置。
type ConfigService struct {
	DB *gorm.DB
	TZ *timezone.Provider
	// OnTimezoneChanged 在站点时区变更后回调，用于重建调度器里的 cron 实例。
	OnTimezoneChanged func()
}

// NewConfigService 构造服务。
func NewConfigService(db *gorm.DB, tz *timezone.Provider) *ConfigService {
	return &ConfigService{DB: db, TZ: tz}
}

// GetAll 读取全部配置，返回 key→value 映射。
//
// 时区未在库里设置过时补上默认值，让设置页始终有一个可编辑的当前值。
func (s *ConfigService) GetAll() (map[string]string, error) {
	var rows []models.Config
	if err := s.DB.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[string]string, len(rows)+1)
	for _, r := range rows {
		out[r.Key] = r.Value
	}
	if _, ok := out[KeyTimezone]; !ok && s.TZ != nil {
		out[KeyTimezone] = s.TZ.Name()
	}
	return out, nil
}

// UpsertMap 批量更新配置。
// value 为空字符串时删除对应 key。
//
// timezone 会先校验：非法时区直接返回校验错误，保证落库的永远是能解析的值。
func (s *ConfigService) UpsertMap(cfg map[string]string) error {
	if v, ok := cfg[KeyTimezone]; ok && v != "" && !timezone.Valid(v) {
		return middleware.NewAppError(middleware.CodeValidationFailed, "未知时区: "+v).WithField(KeyTimezone)
	}

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

	// 时区写到库里之后才生效：先落库，再热更新 Provider 与调度器
	if v, ok := cfg[KeyTimezone]; ok {
		s.applyTimezone(v)
	}
	return nil
}

// LoadTimezone 在启动时把库里的时区应用到 Provider。
//
// 库中没有该配置时保留环境变量 TIMEZONE 给出的默认值。
// 启动阶段调度器尚未开始注册，这里不触发 OnTimezoneChanged，
// 由调用方随后统一 Start + LoadAndRegisterAll。
func (s *ConfigService) LoadTimezone() {
	if s.TZ == nil {
		return
	}
	var row models.Config
	err := s.DB.First(&row, "key = ?", KeyTimezone).Error
	switch {
	case err != nil:
		// 库里还没有时区配置：沿用环境变量给的默认值
	case row.Value == "":
		// 空值等同未配置
	default:
		if setErr := s.TZ.Set(row.Value); setErr != nil {
			slog.Warn("库中时区无效，保留默认值", "timezone", row.Value, "error", setErr)
		}
	}
	slog.Info("站点时区", "timezone", s.TZ.Name())
}

// applyTimezone 把新时区应用到 Provider，并通知调度器重建。
func (s *ConfigService) applyTimezone(name string) {
	if s.TZ == nil {
		return
	}
	if name == "" {
		// 清空配置 = 回到默认时区，仍然要刷新调度器
		name = timezone.Fallback
	}
	if err := s.TZ.Set(name); err != nil {
		slog.Warn("应用时区失败", "timezone", name, "error", err)
		return
	}
	// 这里刻意不改 time.Local：它是全局变量，运行期并发写入是数据竞争。
	// 仅启动阶段（LoadTimezone）同步一次，供日志按天切分等零星用途。
	slog.Info("站点时区已更新", "timezone", s.TZ.Name())
	if s.OnTimezoneChanged != nil {
		s.OnTimezoneChanged()
	}
}
