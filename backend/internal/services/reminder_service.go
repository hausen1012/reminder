// reminder_service 负责提醒的 CRUD/Toggle/Preview/Test，并在写库后同步调度器。
//
// 依赖 SchedulerHook 接口而非直接 import scheduler，避免循环依赖。
package services

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/reminder/backend/internal/middleware"
	"github.com/reminder/backend/internal/models"
	"github.com/reminder/backend/internal/scheduler"
	"github.com/reminder/backend/internal/timezone"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// SchedulerHook 是 ReminderService 同步给调度器的接口（实现：scheduler.Engine，测试可注入 noop）。
type SchedulerHook interface {
	Add(r *models.Reminder) error
	Update(r *models.Reminder) error
	Remove(id uint)
}

// ReminderService 提供提醒的 CRUD 与调度器同步；时区取自站点级 timezone.Provider，落库时间为 UTC。
type ReminderService struct {
	DB         *gorm.DB
	Engine     SchedulerHook
	TZ         *timezone.Provider
	Dispatch   *DispatchService
	ConfirmMgr *ConfirmRetryManager
}

// NewReminderService 构造服务。
func NewReminderService(db *gorm.DB, engine SchedulerHook, tz *timezone.Provider, disp *DispatchService, confirmMgr *ConfirmRetryManager) *ReminderService {
	if tz == nil {
		tz = timezone.New("")
	}
	return &ReminderService{DB: db, Engine: engine, TZ: tz, Dispatch: disp, ConfirmMgr: confirmMgr}
}

// ReminderInput 是 Create / Update 的入参；时区是站点级全局设置，不在此。
type ReminderInput struct {
	Title                   string         `json:"title"`
	Content                 string         `json:"content"`
	ContentFormat           string         `json:"content_format"`
	Calendar                string         `json:"calendar"`      // solar | lunar
	ScheduleType            string         `json:"schedule_type"` // once | interval | cron
	ScheduleSpec            map[string]any `json:"schedule_spec"`
	Enabled                 *bool          `json:"enabled,omitempty"`
	ChannelIDs              []uint         `json:"channel_ids"`
	RequireConfirm          bool           `json:"require_confirm"`
	ConfirmRetryIntervalSec int            `json:"confirm_retry_interval_sec"`
	ConfirmMaxRetries       int            `json:"confirm_max_retries"`
	Source                  string         `json:"source,omitempty"`
	TokenID                 *uint          `json:"token_id,omitempty"`
}

// ReminderView 是返回前端的视图（含 next_fire_at 与绑定的 channel_ids）。
type ReminderView struct {
	models.Reminder
	ScheduleSpec     map[string]any `json:"schedule_spec"`
	ChannelIDs       []uint         `json:"channel_ids"`
	NextFireAtLocal  string         `json:"next_fire_at_local,omitempty"`
	LastFiredAtLocal string         `json:"last_fired_at_local,omitempty"`
}

// Create 新建提醒并写入调度器。
func (s *ReminderService) Create(in ReminderInput) (*ReminderView, error) {
	if in.Calendar == "" {
		in.Calendar = "solar"
	}
	if in.ScheduleType == "" {
		in.ScheduleType = "once"
	}
	if err := s.validate(&in); err != nil {
		return nil, err
	}
	specRaw, err := json.Marshal(in.ScheduleSpec)
	if err != nil {
		return nil, middleware.NewAppError(middleware.CodeValidationFailed, "schedule_spec 不是合法 JSON").WithField("schedule_spec")
	}
	spec, err := scheduler.ParseSpec(specRaw)
	if err != nil {
		return nil, middleware.NewAppError(middleware.CodeValidationFailed, err.Error()).WithField("schedule_spec")
	}
	loc := s.TZ.Get()
	next, err := scheduler.Compute(in.Calendar, in.ScheduleType, spec, time.Now(), loc)
	if err != nil {
		return nil, middleware.NewAppError(middleware.CodeValidationFailed, err.Error()).WithField("schedule_spec")
	}

	source := in.Source
	if source == "" {
		source = "web"
	}
	contentFormat := in.ContentFormat
	if contentFormat == "" {
		contentFormat = "text"
	}
	// 与 Update 保持一致：正文留空时回退为标题。
	if strings.TrimSpace(in.Content) == "" {
		in.Content = strings.TrimSpace(in.Title)
	}
	r := &models.Reminder{
		Title:                   strings.TrimSpace(in.Title),
		Content:                 in.Content,
		ContentFormat:           contentFormat,
		Calendar:                in.Calendar,
		ScheduleType:            in.ScheduleType,
		ScheduleSpec:            datatypes.JSON(specRaw),
		Source:                  source,
		TokenID:                 in.TokenID,
		Enabled:                 true,
		NextFireAt:              models.TimePtr(next),
		RequireConfirm:          in.RequireConfirm,
		ConfirmRetryIntervalSec: in.ConfirmRetryIntervalSec,
		ConfirmMaxRetries:       in.ConfirmMaxRetries,
	}
	if in.Enabled != nil {
		r.Enabled = *in.Enabled
	}
	err = s.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(r).Error; err != nil {
			return err
		}
		return s.replaceChannels(tx, r.ID, in.ChannelIDs)
	})
	if err != nil {
		return nil, err
	}
	if s.Engine != nil {
		if err := s.Engine.Add(r); err != nil {
			// 数据已落库，调度失败只记日志：该提醒将只能依赖兜底扫描
			slog.Error("注册提醒到调度器失败，数据已保存但不会自动触发", "id", r.ID, "title", r.Title, "error", err)
		}
	}
	return s.toView(r, in.ChannelIDs), nil
}

// Update 更新提醒并同步调度器。
func (s *ReminderService) Update(id uint, in ReminderInput) (*ReminderView, error) {
	r, err := s.getOrNotFound(id)
	if err != nil {
		return nil, err
	}
	if err := s.validate(&in); err != nil {
		return nil, err
	}
	specRaw, err := json.Marshal(in.ScheduleSpec)
	if err != nil {
		return nil, middleware.NewAppError(middleware.CodeValidationFailed, "schedule_spec 不是合法 JSON").WithField("schedule_spec")
	}
	spec, err := scheduler.ParseSpec(specRaw)
	if err != nil {
		return nil, middleware.NewAppError(middleware.CodeValidationFailed, err.Error()).WithField("schedule_spec")
	}
	loc := s.TZ.Get()
	next, err := scheduler.Compute(in.Calendar, in.ScheduleType, spec, time.Now(), loc)
	if err != nil {
		return nil, middleware.NewAppError(middleware.CodeValidationFailed, err.Error()).WithField("schedule_spec")
	}

	contentFormat := in.ContentFormat
	if contentFormat == "" {
		contentFormat = "text"
	}
	// 与 Create 一致：正文留空时回退为标题，避免「只改标题」把正文清空
	content := in.Content
	if strings.TrimSpace(content) == "" {
		content = strings.TrimSpace(in.Title)
	}
	updates := map[string]any{
		"title":                      strings.TrimSpace(in.Title),
		"content":                    content,
		"content_format":             contentFormat,
		"calendar":                   in.Calendar,
		"schedule_type":              in.ScheduleType,
		"schedule_spec":              datatypes.JSON(specRaw),
		"require_confirm":            in.RequireConfirm,
		"confirm_retry_interval_sec": in.ConfirmRetryIntervalSec,
		"confirm_max_retries":        in.ConfirmMaxRetries,
	}
	if in.Enabled != nil {
		updates["enabled"] = *in.Enabled
		if !*in.Enabled {
			updates["next_fire_at"] = nil
		} else {
			updates["next_fire_at"] = models.TimePtr(next)
		}
	} else {
		updates["next_fire_at"] = models.TimePtr(next)
	}

	err = s.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.Reminder{}).Where("id = ?", id).Updates(updates).Error; err != nil {
			return err
		}
		return s.replaceChannels(tx, id, in.ChannelIDs)
	})
	if err != nil {
		return nil, err
	}
	// 重读最新状态
	r, err = s.getOrNotFound(id)
	if err != nil {
		return nil, err
	}
	if s.Engine != nil {
		if err := s.Engine.Update(r); err != nil {
			slog.Error("更新调度器失败，改动已保存但调度时间可能未生效", "id", id, "title", r.Title, "error", err)
		}
	}
	if s.ConfirmMgr != nil {
		s.ConfirmMgr.CancelByReminderID(id)
	}
	chIDs, _ := s.channelIDs(id)
	return s.toView(r, chIDs), nil
}

// Get 返回单条提醒视图。
func (s *ReminderService) Get(id uint) (*ReminderView, error) {
	r, err := s.getOrNotFound(id)
	if err != nil {
		return nil, err
	}
	chIDs, err := s.channelIDs(id)
	if err != nil {
		return nil, err
	}
	return s.toView(r, chIDs), nil
}

// ListFilter 列表过滤条件。
type ListFilter struct {
	Source    string // manual | api | ""
	Enabled   *bool
	Search    string
	TokenID   *uint
	Limit     int
	Offset    int
	SortBy    string // created_at | id | next_fire_at
	SortOrder string // asc | desc
}

// List 返回符合条件的提醒分页列表。
func (s *ReminderService) List(f ListFilter) ([]*ReminderView, int64, error) {
	q := s.DB.Model(&models.Reminder{})
	if f.Source != "" && f.Source != "all" {
		q = q.Where("source = ?", f.Source)
	}
	if f.TokenID != nil {
		q = q.Where("token_id = ?", *f.TokenID)
	}
	if f.Enabled != nil {
		q = q.Where("enabled = ?", *f.Enabled)
	}
	if s := strings.TrimSpace(f.Search); s != "" {
		like := "%" + s + "%"
		q = q.Where("title LIKE ? OR content LIKE ?", like, like)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if f.Limit <= 0 {
		f.Limit = 50
	}
	if f.Limit > 200 {
		f.Limit = 200
	}

	orderClause := buildOrderClause(f.SortBy, f.SortOrder, "created_at DESC",
		[]string{"id", "created_at", "next_fire_at", "title"})
	// next_fire_at 是整数毫秒列，直接排序即为时间序
	var rows []models.Reminder
	if err := q.Order(orderClause).Limit(f.Limit).Offset(f.Offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	views, err := s.viewsOf(rows)
	if err != nil {
		return nil, 0, err
	}
	return views, total, nil
}

// Upcoming 返回未来 within 时间内待触发的 enabled 提醒。
func (s *ReminderService) Upcoming(within time.Duration, limit int) ([]*ReminderView, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}
	// next_fire_at 是整数毫秒列：范围比较与排序都走索引
	cutoff := time.Now().Add(within)
	var rows []models.Reminder
	if err := s.DB.Where("enabled = ? AND next_fire_at IS NOT NULL AND next_fire_at <= ?", true, models.TimeOf(cutoff)).
		Order("next_fire_at ASC").
		Limit(limit).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	return s.viewsOf(rows)
}

// UpcomingBetween 返回指定时间范围内待触发的 enabled 提醒（无 limit 限制）。
func (s *ReminderService) UpcomingBetween(from, to time.Time, limit int) ([]*ReminderView, error) {
	var rows []models.Reminder
	q := s.DB.Where("enabled = ? AND next_fire_at IS NOT NULL AND next_fire_at BETWEEN ? AND ?",
		true, models.TimeOf(from), models.TimeOf(to)).
		Order("next_fire_at ASC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	return s.viewsOf(rows)
}

// Delete 硬删除一条提醒并从调度器移除。
func (s *ReminderService) Delete(id uint) error {
	r, err := s.getOrNotFound(id)
	if err != nil {
		return err
	}
	if err := s.DB.Transaction(func(tx *gorm.DB) error {
		// 清理关联表
		if err := tx.Where("reminder_id = ?", id).Delete(&models.ReminderChannel{}).Error; err != nil {
			return err
		}
		return tx.Unscoped().Delete(r).Error
	}); err != nil {
		return err
	}
	if s.Engine != nil {
		s.Engine.Remove(id)
	}
	if s.ConfirmMgr != nil {
		s.ConfirmMgr.CancelByReminderID(id)
	}
	return nil
}

// BatchDelete 批量硬删除提醒并从调度器移除；关联与主体的删除在同一事务内。
func (s *ReminderService) BatchDelete(ids []uint) error {
	if len(ids) == 0 {
		return nil
	}
	if err := s.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("reminder_id IN ?", ids).Delete(&models.ReminderChannel{}).Error; err != nil {
			return err
		}
		return tx.Where("id IN ?", ids).Unscoped().Delete(&models.Reminder{}).Error
	}); err != nil {
		return err
	}
	// 事务提交后再清理调度器与确认重试，避免事务内访问 DB 造成 SQLite 死锁
	for _, id := range ids {
		if s.Engine != nil {
			s.Engine.Remove(id)
		}
		if s.ConfirmMgr != nil {
			s.ConfirmMgr.CancelByReminderID(id)
		}
	}
	return nil
}

// Toggle 启用 / 禁用。
func (s *ReminderService) Toggle(id uint) (*ReminderView, error) {
	r, err := s.getOrNotFound(id)
	if err != nil {
		return nil, err
	}
	r.Enabled = !r.Enabled
	updates := map[string]any{"enabled": r.Enabled}
	if r.Enabled {
		// 重新启用时重新计算 next_fire_at
		spec, perr := scheduler.ParseSpec(r.ScheduleSpec)
		if perr == nil {
			if next, cerr := scheduler.Compute(r.Calendar, r.ScheduleType, spec, time.Now(), s.TZ.Get()); cerr == nil {
				updates["next_fire_at"] = models.TimePtr(next)
				r.NextFireAt = models.TimePtr(next)
			} else {
				// 计算失败时回退到旧值，保持禁用
				r.Enabled = false
				updates["enabled"] = false
				slog.Warn("重新计算下次触发时间失败，保持禁用", "id", id, "error", cerr)
			}
		} else {
			r.Enabled = false
			updates["enabled"] = false
			slog.Warn("解析 schedule_spec 失败，保持禁用", "id", id, "error", perr)
		}
	} else {
		// 禁用时清空下次触发时间
		updates["next_fire_at"] = nil
		r.NextFireAt = nil
	}
	if err := s.DB.Model(&models.Reminder{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return nil, err
	}
	if s.Engine != nil {
		if r.Enabled {
			if err := s.Engine.Add(r); err != nil {
				slog.Error("启用提醒时注册调度器失败，提醒已启用但不会自动触发", "id", id, "title", r.Title, "error", err)
			}
		} else {
			s.Engine.Remove(id)
		}
	}
	if !r.Enabled && s.ConfirmMgr != nil {
		s.ConfirmMgr.CancelByReminderID(id)
	}
	chIDs, _ := s.channelIDs(id)
	return s.toView(r, chIDs), nil
}

// Preview 计算给定 spec 的下 n 次触发时间（时区取当前站点时区）。
func (s *ReminderService) Preview(calendar, scheduleType string, specMap map[string]any, n int) ([]time.Time, error) {
	if n <= 0 {
		n = 5
	}
	if n > 20 {
		n = 20
	}
	raw, _ := json.Marshal(specMap)
	spec, err := scheduler.ParseSpec(raw)
	if err != nil {
		return nil, middleware.NewAppError(middleware.CodeValidationFailed, err.Error())
	}
	from := time.Now()
	return scheduler.PreviewSolar(calendar, scheduleType, spec, from, s.TZ.Get(), n)
}

// Now 返回当前站点时区下的时间，供 handler 计算「今天」的边界。
func (s *ReminderService) Now() time.Time {
	return time.Now().In(s.TZ.Get())
}

// TestDryRun 用表单数据试发提醒，不写 delivery_log / delivery_attempt。
func (s *ReminderService) TestDryRun(ctx context.Context, title, content, contentFormat string, channelIDs []uint) error {
	if s.Dispatch == nil {
		return errors.New("dispatch 未初始化")
	}
	if len(channelIDs) == 0 {
		return middleware.NewAppError(middleware.CodeValidationFailed, "至少选择一个通知渠道")
	}
	if strings.TrimSpace(title) == "" {
		return middleware.NewAppError(middleware.CodeValidationFailed, "标题必填")
	}
	// 加载绑定的 enabled 通道
	channels, err := s.loadChannelsByIDs(channelIDs)
	if err != nil {
		return err
	}
	if len(channels) == 0 {
		return middleware.NewAppError(middleware.CodeValidationFailed, "没有可用的通知渠道")
	}

	now := time.Now()
	vars := buildVars(&models.Reminder{
		Title:      title,
		Content:    content,
		Source:     "web",
		NextFireAt: models.TimePtrOf(now),
	}, now, now, s.TZ.Get())

	rendered := buildRenderedMessage(title, content, contentFormat, vars)
	return s.Dispatch.DryRun(ctx, channels, rendered)
}

// loadChannelsByIDs 根据 ID 列表加载通道。
func (s *ReminderService) loadChannelsByIDs(ids []uint) ([]*models.Channel, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var rows []models.Channel
	if err := s.DB.Where("id IN ?", ids).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]*models.Channel, 0, len(rows))
	for i := range rows {
		out = append(out, &rows[i])
	}
	return out, nil
}

// --- internal helpers ---

func (s *ReminderService) validate(in *ReminderInput) error {
	if strings.TrimSpace(in.Title) == "" {
		return middleware.NewAppError(middleware.CodeValidationFailed, "标题必填").WithField("title")
	}
	if len([]rune(in.Title)) > 200 {
		return middleware.NewAppError(middleware.CodeValidationFailed, "标题最长 200 字符").WithField("title")
	}
	if in.Calendar != "solar" && in.Calendar != "lunar" {
		return middleware.NewAppError(middleware.CodeValidationFailed, "未知 calendar").WithField("calendar")
	}
	switch in.ScheduleType {
	case "once", "interval", "cron":
	default:
		return middleware.NewAppError(middleware.CodeValidationFailed, "未知 schedule_type").WithField("schedule_type")
	}
	if in.Calendar == "lunar" && in.ScheduleType == "cron" {
		return middleware.NewAppError(middleware.CodeValidationFailed, "lunar 不支持 cron").WithField("schedule_type")
	}
	if in.ContentFormat != "" && in.ContentFormat != "text" && in.ContentFormat != "markdown" && in.ContentFormat != "html" {
		return middleware.NewAppError(middleware.CodeValidationFailed, "content_format 仅支持 text / markdown / html").WithField("content_format")
	}
	if in.ScheduleSpec == nil {
		return middleware.NewAppError(middleware.CodeValidationFailed, "schedule_spec 必填").WithField("schedule_spec")
	}
	if in.RequireConfirm {
		if in.ConfirmRetryIntervalSec < 60 {
			return middleware.NewAppError(middleware.CodeValidationFailed, "重发间隔最小 60 秒").WithField("confirm_retry_interval_sec")
		}
		if in.ConfirmMaxRetries < 1 {
			return middleware.NewAppError(middleware.CodeValidationFailed, "最大重试次数至少 1").WithField("confirm_max_retries")
		}
	}
	if err := s.validateChannels(in.ChannelIDs); err != nil {
		return err
	}
	return nil
}

func (s *ReminderService) validateChannels(ids []uint) error {
	if len(ids) == 0 {
		return nil
	}
	dedup := make([]uint, 0, len(ids))
	seen := make(map[uint]bool, len(ids))
	for _, id := range ids {
		if id == 0 {
			return middleware.NewAppError(middleware.CodeValidationFailed, "指定的通知渠道不存在").WithField("channel_ids")
		}
		if !seen[id] {
			seen[id] = true
			dedup = append(dedup, id)
		}
	}
	var count int64
	if err := s.DB.Model(&models.Channel{}).Where("id IN ?", dedup).Count(&count).Error; err != nil {
		return err
	}
	if count != int64(len(dedup)) {
		return middleware.NewAppError(middleware.CodeValidationFailed, "指定的通知渠道不存在").WithField("channel_ids")
	}
	return nil
}

func (s *ReminderService) replaceChannels(tx *gorm.DB, reminderID uint, ids []uint) error {
	if err := tx.Where("reminder_id = ?", reminderID).Delete(&models.ReminderChannel{}).Error; err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	rows := make([]models.ReminderChannel, 0, len(ids))
	seen := map[uint]bool{}
	for _, id := range ids {
		if id == 0 || seen[id] {
			continue
		}
		seen[id] = true
		rows = append(rows, models.ReminderChannel{ReminderID: reminderID, ChannelID: id})
	}
	if len(rows) == 0 {
		return nil
	}
	return tx.Create(&rows).Error
}

func (s *ReminderService) channelIDs(reminderID uint) ([]uint, error) {
	var ids []uint
	err := s.DB.Model(&models.ReminderChannel{}).
		Where("reminder_id = ?", reminderID).
		Order("channel_id ASC").
		Pluck("channel_id", &ids).Error
	if err != nil {
		return nil, err
	}
	return ids, nil
}

// channelIDsBatch 一次查出多条提醒绑定的通道，避免列表接口 N+1 查询。
func (s *ReminderService) channelIDsBatch(reminderIDs []uint) (map[uint][]uint, error) {
	out := make(map[uint][]uint, len(reminderIDs))
	if len(reminderIDs) == 0 {
		return out, nil
	}
	type pair struct {
		ReminderID uint
		ChannelID  uint
	}
	var rows []pair
	if err := s.DB.Model(&models.ReminderChannel{}).
		Select("reminder_id, channel_id").
		Where("reminder_id IN ?", reminderIDs).
		Order("reminder_id ASC, channel_id ASC").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.ReminderID] = append(out[row.ReminderID], row.ChannelID)
	}
	return out, nil
}

// viewsOf 把一批提醒转成视图，通道绑定一次性批量查出（避免 N+1）。
func (s *ReminderService) viewsOf(rows []models.Reminder) ([]*ReminderView, error) {
	ids := make([]uint, 0, len(rows))
	for i := range rows {
		ids = append(ids, rows[i].ID)
	}
	chMap, err := s.channelIDsBatch(ids)
	if err != nil {
		return nil, err
	}
	views := make([]*ReminderView, 0, len(rows))
	for i := range rows {
		views = append(views, s.toView(&rows[i], chMap[rows[i].ID]))
	}
	return views, nil
}

func (s *ReminderService) getOrNotFound(id uint) (*models.Reminder, error) {
	var r models.Reminder
	if err := s.DB.First(&r, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, middleware.NewAppError(middleware.CodeNotFound, "提醒不存在")
		}
		return nil, err
	}
	return &r, nil
}

func (s *ReminderService) toView(r *models.Reminder, chIDs []uint) *ReminderView {
	specMap := map[string]any{}
	if len(r.ScheduleSpec) > 0 {
		_ = json.Unmarshal(r.ScheduleSpec, &specMap)
	}
	if chIDs == nil {
		chIDs = []uint{}
	}
	v := &ReminderView{
		Reminder:     *r,
		ScheduleSpec: specMap,
		ChannelIDs:   chIDs,
	}
	loc := s.TZ.Get()
	if r.NextFireAt != nil {
		v.NextFireAtLocal = r.NextFireAt.In(loc).Format("2006-01-02 15:04")
	}
	if r.LastFiredAt != nil {
		v.LastFiredAtLocal = r.LastFiredAt.In(loc).Format("2006-01-02 15:04")
	}
	return v
}
