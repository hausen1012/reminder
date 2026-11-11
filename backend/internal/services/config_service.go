package services

import (
	"errors"
	"log/slog"
	"strings"

	"github.com/reminder/backend/internal/middleware"
	"github.com/reminder/backend/internal/timezone"
	"gorm.io/gorm"
)

// KeyTimezone 是站点时区在 config 表中的 key。
const KeyTimezone = "timezone"

// KeySiteURL 是站点对外地址在 config 表中的 key，决定确认链接用哪个域名拼接。
const KeySiteURL = "site_url"

// 站点配置的 key 白名单。
//
// /api/config 是给面板用的通用接口，不设边界就能写入 ai_api_key_enc 之类的密钥。
// AI 设置只走 /api/ai/settings。
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
	// SiteURLFallback 是环境变量 SITE_URL 的初始值，库里没有 site_url 时生效。
	SiteURLFallback string
	// OnTimezoneChanged 在站点时区变更后回调，用于重建调度器里的 cron 实例。
	OnTimezoneChanged func()
}

// NewConfigService 构造服务。
func NewConfigService(db *gorm.DB, tz *timezone.Provider) *ConfigService {
	return &ConfigService{DB: db, TZ: tz}
}

// GetAll 读取白名单内的站点配置；时区与站点地址未落库时回显当前生效值。
func (s *ConfigService) GetAll() (map[string]string, error) {
	all, err := configStore{s.DB}.all()
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(allowedConfigKeys))
	for k, v := range all {
		if _, ok := allowedConfigKeys[k]; !ok {
			continue
		}
		out[k] = v
	}
	if _, ok := out[KeyTimezone]; !ok && s.TZ != nil {
		out[KeyTimezone] = s.TZ.Name()
	}
	// 对外地址同理：库里没设置过时回显环境变量初始值
	if _, ok := out[KeySiteURL]; !ok && s.SiteURLFallback != "" {
		out[KeySiteURL] = s.SiteURLFallback
	}
	return out, nil
}

// UpsertMap 批量更新站点配置，值为空串表示删除该 key。
//
// 写库前整体校验白名单与时区，避免写了一半才发现非法；时区只有真正变化才触发调度器重建。
func (s *ConfigService) UpsertMap(cfg map[string]string) error {
	for k := range cfg {
		if _, ok := allowedConfigKeys[k]; !ok {
			return middleware.NewAppError(middleware.CodeValidationFailed, "不支持的配置项: "+k).WithField(k)
		}
	}
	if v, ok := cfg[KeyTimezone]; ok && v != "" && !timezone.Valid(v) {
		return middleware.NewAppError(middleware.CodeValidationFailed, "未知时区: "+v).WithField(KeyTimezone)
	}
	// 地址必须是 http(s)，并去掉结尾斜杠；只改副本，不动调用方传入的 map
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
		if err := (configStore{s.DB}).set(k, v); err != nil {
			return err
		}
	}

	// 时区写到库里之后才生效：先落库，再热更新 Provider 与调度器
	if v, ok := cfg[KeyTimezone]; ok {
		s.applyTimezone(v)
	}
	return nil
}

// normalizeSiteURL 校验站点地址必须以 http(s) 开头，并去掉首尾空白与结尾斜杠；
// 空串表示清除配置，不在此校验。
func normalizeSiteURL(raw string) (string, error) {
	v := strings.TrimSpace(raw)
	lower := strings.ToLower(v)
	if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
		return "", errors.New("站点地址必须以 http:// 或 https:// 开头")
	}
	return strings.TrimRight(v, "/"), nil
}

// siteURL 返回拼接确认链接用的站点地址：库里优先，其次环境变量，最后默认值。
// 每次现读库，改完站点地址立即对之后的发送生效。
func siteURL(db *gorm.DB, fallback string) string {
	if v, ok, err := (configStore{db}).get(KeySiteURL); err == nil && ok && v != "" {
		// 库里的值可能没经过校验，这里同样去掉结尾斜杠
		return strings.TrimRight(strings.TrimSpace(v), "/")
	}
	base := strings.TrimRight(strings.TrimSpace(fallback), "/")
	if base == "" {
		return "http://localhost:8765"
	}
	return base
}

// LoadTimezone 在启动时把库里的时区应用到 Provider；库里没有时保留环境变量默认值。
//
// 启动阶段不触发 OnTimezoneChanged，也不重算 next_fire_at（由调用方随后统一 Start + LoadAndRegisterAll）。
func (s *ConfigService) LoadTimezone() {
	if s.TZ == nil {
		return
	}
	row, ok, err := (configStore{s.DB}).get(KeyTimezone)
	switch {
	case err != nil:
		// 读库失败：沿用环境变量给的默认值
	case !ok || row == "":
		// 库里还没有时区配置：空值等同未配置
	default:
		if setErr := s.TZ.Set(row); setErr != nil {
			slog.Warn("库中时区无效，保留默认值", "timezone", row, "error", setErr)
		}
	}
	slog.Info("站点时区", "timezone", s.TZ.Name())
}

// applyTimezone 把新时区应用到 Provider 并通知调度器重建；时区未变化时直接返回。
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
	// 不改 time.Local：全局变量，运行期写入是数据竞争；启动时已同步过一次
	slog.Info("站点时区已更新", "timezone", s.TZ.Name())
	if s.OnTimezoneChanged != nil {
		s.OnTimezoneChanged()
	}
}
