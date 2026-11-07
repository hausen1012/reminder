package services

import (
	"errors"
	"log/slog"
	"strings"

	"github.com/reminder/backend/internal/middleware"
	"github.com/reminder/backend/internal/models"
	"github.com/reminder/backend/internal/timezone"
	"gorm.io/gorm"
)

// KeyTimezone 是站点时区在 config 表中的 key。
const KeyTimezone = "timezone"

// KeySiteURL 是站点对外地址在 config 表中的 key。
//
// 它决定确认链接（/c/:token）用哪个域名拼接：用户点开链接的通常是手机，
// 地址必须手机可达。环境变量 SITE_URL 只在库里没有该配置时作为回退值。
const KeySiteURL = "site_url"

// 站点配置的 key 白名单。
//
// config 表本身是通用的键值表，但 /api/config 是一个给面板用的通用接口：
// 若不设边界，调用方可以借它读写任意 key——最严重的是写入 ai_api_key_enc，
// 绕过 /api/ai/settings 的加密与校验路径，把 AI 密钥写成明文或垃圾值；
// GetAll 全量返回还会把这类密钥带到浏览器。
//
// AI 设置（ai_ 前缀）只走 /api/ai/settings，不在此列。
var allowedConfigKeys = map[string]struct{}{
	"app_name":  {},
	"logo_svg":  {},
	KeyTimezone: {},
	KeySiteURL:  {},
}

// ConfigService 管理站点配置。
type ConfigService struct {
	DB *gorm.DB
	TZ *timezone.Provider
	// SiteURLFallback 是环境变量 SITE_URL 给出的初始值：
	// 库里没有 site_url 时，它就是实际生效的地址，也用于设置页回显。
	SiteURLFallback string
	// OnTimezoneChanged 在站点时区变更后回调，用于重建调度器里的 cron 实例。
	OnTimezoneChanged func()
}

// NewConfigService 构造服务。
func NewConfigService(db *gorm.DB, tz *timezone.Provider) *ConfigService {
	return &ConfigService{DB: db, TZ: tz}
}

// GetAll 读取允许公开的站点配置，返回 key→value 映射。
//
// 只返回白名单里的 key：表里可能还有 AI 设置等不该经通用接口外泄的内容。
// 时区未在库里设置过时补上默认值，让设置页始终有一个可编辑的当前值。
func (s *ConfigService) GetAll() (map[string]string, error) {
	var rows []models.Config
	if err := s.DB.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[string]string, len(allowedConfigKeys))
	for _, r := range rows {
		if _, ok := allowedConfigKeys[r.Key]; !ok {
			continue
		}
		out[r.Key] = r.Value
	}
	if _, ok := out[KeyTimezone]; !ok && s.TZ != nil {
		out[KeyTimezone] = s.TZ.Name()
	}
	// 对外地址同理：库里没设置过时回显环境变量给的初始值，
	// 设置页才有可编辑的当前值，而不是空白框。
	if _, ok := out[KeySiteURL]; !ok && s.SiteURLFallback != "" {
		out[KeySiteURL] = s.SiteURLFallback
	}
	return out, nil
}

// UpsertMap 批量更新站点配置。
// value 为空字符串时删除对应 key。
//
// 只接受白名单里的 key，其它一律拒绝：见 allowedConfigKeys 的说明。
// 先在写库之前整体校验，避免「写了一半才发现某个 key 非法」。
// timezone 会先校验：非法时区直接返回校验错误，保证落库的永远是能解析的值。
// 时区只有在值确实变化时才触发重建：设置页每次保存都会把全部配置回传，
// 没改时区却重注册全部提醒会产生无谓的调度抖动。
func (s *ConfigService) UpsertMap(cfg map[string]string) error {
	for k := range cfg {
		if _, ok := allowedConfigKeys[k]; !ok {
			return middleware.NewAppError(middleware.CodeValidationFailed, "不支持的配置项: "+k).WithField(k)
		}
	}
	if v, ok := cfg[KeyTimezone]; ok && v != "" && !timezone.Valid(v) {
		return middleware.NewAppError(middleware.CodeValidationFailed, "未知时区: "+v).WithField(KeyTimezone)
	}
	// 地址必须是 http(s)，并去掉结尾斜杠（否则会拼出 https://a.com//c/token）。
	// 只改内存副本，不动调用方传进来的 map。
	toWrite := make(map[string]string, len(cfg))
	for k, v := range cfg {
		toWrite[k] = v
	}
	if v, ok := toWrite[KeySiteURL]; ok && v != "" {
		normalized, err := normalizeSiteURL(v)
		if err != nil {
			return middleware.NewAppError(middleware.CodeValidationFailed, err.Error()).WithField(KeySiteURL)
		}
		toWrite[KeySiteURL] = normalized
	}

	for k, v := range toWrite {
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

// normalizeSiteURL 校验并规整站点地址：必须是 http(s)，去掉首尾空白与结尾斜杠。
//
// 结尾斜杠会让拼接结果变成 https://a.com//c/token。
// 空串不在校验范围内：它表示「清除配置」，由调用方回退到环境变量。
func normalizeSiteURL(raw string) (string, error) {
	v := strings.TrimSpace(raw)
	lower := strings.ToLower(v)
	if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
		return "", errors.New("站点地址必须以 http:// 或 https:// 开头")
	}
	return strings.TrimRight(v, "/"), nil
}

// siteURL 返回拼接确认链接用的站点地址。
//
// 优先取设置页保存的值（config 表 key=site_url），没有时回退到环境变量 SITE_URL，
// 再回退到默认地址。每次现读库而不是启动时快照：这个地址常常是部署完才发现填错的，
// 用户改完就该对之后的发送生效，不该要求重启进程。
// 现读的代价是一条按主键查的 SELECT，而它只发生在渲染确认链接时（投递或日志详情），
// 频率远低于调度本身。
func siteURL(db *gorm.DB, fallback string) string {
	var row models.Config
	if err := db.First(&row, "key = ?", KeySiteURL).Error; err == nil && row.Value != "" {
		// 库里的值可能是校验规则上线前写入的，这里同样去掉结尾斜杠
		return strings.TrimRight(strings.TrimSpace(row.Value), "/")
	}
	base := strings.TrimRight(strings.TrimSpace(fallback), "/")
	if base == "" {
		return "http://localhost:8765"
	}
	return base
}

// LoadTimezone 在启动时把库里的时区应用到 Provider。
//
// 库中没有该配置时保留环境变量 TIMEZONE 给出的默认值。
// 启动阶段调度器尚未开始注册，这里不触发 OnTimezoneChanged，
// 由调用方随后统一 Start + LoadAndRegisterAll。
//
// 启动时不重算 next_fire_at：库里有配置时，重启前后时区必然一致，重算纯属多余；
// 库里没有配置而 TIMEZONE 被改动的情况交给 sweeper 按容忍窗口处理
// （超出窗口标 expired，用户能在日志里看到，不会静默消失）。
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
//
// 传入值就是最终值（空字符串已在此按回退时区处理）。
// 与当前生效时区相同时直接返回：重复保存同一个时区不该重建调度器。
func (s *ConfigService) applyTimezone(name string) {
	if s.TZ == nil {
		return
	}
	if name == "" {
		// 清空配置 = 回到默认时区，仍然要刷新调度器
		name = timezone.Fallback
	}
	if s.TZ.Name() == name {
		slog.Debug("站点时区未变化，跳过调度器重建", "timezone", name)
		return
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
