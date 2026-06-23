// reminder_service 负责提醒的 CRUD/Toggle/Preview/Test，
// 并在写库后同步通知调度器（Engine.Add/Update/Remove）。
//
// 与 scheduler 的解耦：本服务依赖 SchedulerHook 接口而不是直接 import scheduler，
// 这样 scheduler 在调度回调里依赖 services 时不会产生循环。
package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/bedrock/backend/internal/middleware"
	"github.com/bedrock/backend/internal/models"
	"github.com/bedrock/backend/internal/notifier"
	"github.com/bedrock/backend/internal/scheduler"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// SchedulerHook 是 ReminderService 同步给调度器的接口。
//
// 实现：scheduler.Engine。允许测试时注入 noop。
type SchedulerHook interface {
	Add(r *models.Reminder) error
	Update(r *models.Reminder) error
	Remove(id uint)
}

// ReminderService 提供提醒的 CRUD 与调度器同步。
type ReminderService struct {
	DB         *gorm.DB
	Engine     SchedulerHook
	Loc        *time.Location
	Dispatch   *DispatchService
	ConfirmMgr *ConfirmRetryManager
}

// NewReminderService 构造服务。
func NewReminderService(db *gorm.DB, engine SchedulerHook, loc *time.Location, disp *DispatchService, confirmMgr *ConfirmRetryManager) *ReminderService {
	return &ReminderService{DB: db, Engine: engine, Loc: loc, Dispatch: disp, ConfirmMgr: confirmMgr}
}

// ReminderInput 是 Create / Update 的入参。
type ReminderInput struct {
	Title                   string         `json:"title"`
	Content                 string         `json:"content"`
	ContentFormat           string         `json:"content_format"`
	Calendar                string         `json:"calendar"`      // solar | lunar
	ScheduleType            string         `json:"schedule_type"` // once | interval | cron
	ScheduleSpec            map[string]any `json:"schedule_spec"`
	Timezone                string         `json:"timezone"`
	Enabled                 *bool          `json:"enabled,omitempty"`
	ChannelIDs              []uint         `json:"channel_ids"`
	RequireConfirm          bool           `json:"require_confirm"`
	ConfirmRetryIntervalSec int            `json:"confirm_retry_interval_sec"`
	ConfirmMaxRetries       int            `json:"confirm_max_retries"`
	Source                  string         `json:"source,omitempty"`
	APIKeyID                *uint          `json:"api_key_id,omitempty"`
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
	specRaw, _ := json.Marshal(in.ScheduleSpec)
	spec, err := scheduler.ParseSpec(specRaw)
	if err != nil {
		return nil, middleware.NewAppError(middleware.CodeValidationFailed, err.Error()).WithField("schedule_spec")
	}
	loc := s.locFor(in.Timezone)
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
	r := &models.Reminder{
		Title:                   strings.TrimSpace(in.Title),
		Content:                 in.Content,
		ContentFormat:           contentFormat,
		Calendar:                in.Calendar,
		ScheduleType:            in.ScheduleType,
		ScheduleSpec:            datatypes.JSON(specRaw),
		Timezone:                in.Timezone,
		Source:                  in.Source,
		APIKeyID:                in.APIKeyID,
		Enabled:                 true,
		NextFireAt:              next,
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
			// 调度器失败不应导致 CRUD 失败：用户保存的数据已落库
			fmt.Printf("[reminder] 注册到调度器失败 id=%d: %v\n", r.ID, err)
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
	specRaw, _ := json.Marshal(in.ScheduleSpec)
	spec, err := scheduler.ParseSpec(specRaw)
	if err != nil {
		return nil, middleware.NewAppError(middleware.CodeValidationFailed, err.Error()).WithField("schedule_spec")
	}
	loc := s.locFor(in.Timezone)
	next, err := scheduler.Compute(in.Calendar, in.ScheduleType, spec, time.Now(), loc)
	if err != nil {
		return nil, middleware.NewAppError(middleware.CodeValidationFailed, err.Error()).WithField("schedule_spec")
	}

	contentFormat := in.ContentFormat
	if contentFormat == "" {
		contentFormat = "text"
	}
	updates := map[string]any{
		"title":                      strings.TrimSpace(in.Title),
		"content":                    in.Content,
		"content_format":             contentFormat,
		"calendar":                   in.Calendar,
		"schedule_type":              in.ScheduleType,
		"schedule_spec":              datatypes.JSON(specRaw),
		"timezone":                   in.Timezone,
		"require_confirm":            in.RequireConfirm,
		"confirm_retry_interval_sec": in.ConfirmRetryIntervalSec,
		"confirm_max_retries":        in.ConfirmMaxRetries,
	}
	if in.Enabled != nil {
		updates["enabled"] = *in.Enabled
		if !*in.Enabled {
			updates["next_fire_at"] = nil
		} else {
			updates["next_fire_at"] = next
		}
	} else {
		updates["next_fire_at"] = next
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
			fmt.Printf("[reminder] 更新调度器失败 id=%d: %v\n", id, err)
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
	APIKeyID  *uint
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
	if f.APIKeyID != nil {
		q = q.Where("api_key_id = ?", *f.APIKeyID)
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

	orderClause := "created_at DESC"
	if f.SortBy != "" {
		orderClause = f.SortBy
		if f.SortOrder == "asc" {
			orderClause += " ASC"
		} else {
			orderClause += " DESC"
		}
	}
	var rows []models.Reminder
	if err := q.Order(orderClause).Limit(f.Limit).Offset(f.Offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	views := make([]*ReminderView, 0, len(rows))
	for i := range rows {
		chIDs, _ := s.channelIDs(rows[i].ID)
		views = append(views, s.toView(&rows[i], chIDs))
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
	cutoff := time.Now().Add(within)
	var rows []models.Reminder
	if err := s.DB.Where("enabled = ? AND deleted_at IS NULL AND next_fire_at IS NOT NULL AND next_fire_at <= ?", true, cutoff).
		Order("next_fire_at ASC").
		Limit(limit).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	views := make([]*ReminderView, 0, len(rows))
	for i := range rows {
		chIDs, _ := s.channelIDs(rows[i].ID)
		views = append(views, s.toView(&rows[i], chIDs))
	}
	return views, nil
}

// UpcomingBetween 返回指定时间范围内待触发的 enabled 提醒（无 limit 限制）。
func (s *ReminderService) UpcomingBetween(from, to time.Time, limit int) ([]*ReminderView, error) {
	var rows []models.Reminder
	q := s.DB.Where("enabled = ? AND deleted_at IS NULL AND next_fire_at IS NOT NULL AND next_fire_at BETWEEN ? AND ?", true, from, to).
		Order("next_fire_at ASC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	views := make([]*ReminderView, 0, len(rows))
	for i := range rows {
		chIDs, _ := s.channelIDs(rows[i].ID)
		views = append(views, s.toView(&rows[i], chIDs))
	}
	return views, nil
}

// Delete 硬删除一条提醒并从调度器移除。
func (s *ReminderService) Delete(id uint) error {
	r, err := s.getOrNotFound(id)
	if err != nil {
		return err
	}
	if err := s.DB.Unscoped().Delete(r).Error; err != nil {
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
			loc := s.locFor(r.Timezone)
			if next, cerr := scheduler.Compute(r.Calendar, r.ScheduleType, spec, time.Now(), loc); cerr == nil {
				updates["next_fire_at"] = next
				r.NextFireAt = next
			}
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
			_ = s.Engine.Add(r)
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

// Preview 计算给定 spec 的下 n 次触发时间。
func (s *ReminderService) Preview(calendar, scheduleType string, specMap map[string]any, tz string, n int) ([]time.Time, error) {
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
	loc := s.locFor(tz)
	from := time.Now()
	return scheduler.PreviewSolar(calendar, scheduleType, spec, from, loc, n)
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
		Title:       title,
		Content:     content,
		NextFireAt:  &now,
	}, now, now, s.Loc)

	body := notifier.Render(content, vars)
	rendered := notifier.Message{
		Subject: notifier.Render(title, vars),
		Body:    body,
		Format:  contentFormat,
		Vars:    vars,
	}
	return s.Dispatch.DryRun(ctx, channels, rendered)
}

// loadChannelsByIDs 根据 ID 列表加载 enabled 通道。
func (s *ReminderService) loadChannelsByIDs(ids []uint) ([]*models.Channel, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var rows []models.Channel
	if err := s.DB.Where("id IN ? AND enabled = ?", ids, true).Find(&rows).Error; err != nil {
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

func (s *ReminderService) locFor(tz string) *time.Location {
	if tz != "" {
		if loc, err := time.LoadLocation(tz); err == nil {
			return loc
		}
	}
	if s.Loc != nil {
		return s.Loc
	}
	return time.Local
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
	loc := s.locFor(r.Timezone)
	if r.NextFireAt != nil {
		v.NextFireAtLocal = r.NextFireAt.In(loc).Format("2006-01-02 15:04")
	}
	if r.LastFiredAt != nil {
		v.LastFiredAtLocal = r.LastFiredAt.In(loc).Format("2006-01-02 15:04")
	}
	return v
}
