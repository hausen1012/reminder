// log_service 提供日志查询与清理功能。
//
// 日志是调度触发与 Dispatch 运行的结果沉淀，不可修改仅可查询和按时间范围删除。
package services

import (
	"errors"
	"time"

	"github.com/bedrock/backend/internal/middleware"
	"github.com/bedrock/backend/internal/models"
	"gorm.io/gorm"
)

// LogService 负责 DeliveryLog 的查询与清理。
type LogService struct {
	DB          *gorm.DB
	PublicURL   string // 用于拼接 confirm_url
}

// NewLogService 构造日志服务。
func NewLogService(db *gorm.DB, publicURL ...string) *LogService {
	url := "http://localhost:8080"
	if len(publicURL) > 0 && publicURL[0] != "" {
		url = publicURL[0]
	}
	return &LogService{DB: db, PublicURL: url}
}

// LogFilter 日志列表过滤条件。
type LogFilter struct {
	ReminderID uint   // 指定提醒
	Status     string // pending | success | partial | failed | expired
	Source     string
	Search     string // 搜索日志标题
	Limit      int
	Offset     int
}

// LogView 是返回前端的日志视图（含关联的 remind 基础信息）。
type LogView struct {
	models.DeliveryLog
	ReminderTitle   string `json:"reminder_title"`
	ReminderDeleted bool   `json:"reminder_deleted"`

	// 本日志关联的 attempts（仅详情接口加载）
	Attempts []models.DeliveryAttempt `json:"attempts,omitempty"`

	// 确认链接 URL（仅需要确认且有 token 的日志）
	ConfirmURL string `json:"confirm_url,omitempty"`
}

// List 查询日志列表，返回视图切片与总数。
func (s *LogService) List(f LogFilter) ([]*LogView, int64, error) {
	q := s.DB.Model(&models.DeliveryLog{}).
		Select("delivery_logs.*, COALESCE(r.title, delivery_logs.title) AS reminder_title, r.deleted_at IS NOT NULL AS reminder_deleted").
		Joins("LEFT JOIN reminders r ON r.id = delivery_logs.reminder_id")

	if f.ReminderID > 0 {
		q = q.Where("delivery_logs.reminder_id = ?", f.ReminderID)
	}
	if f.Status != "" {
		q = q.Where("delivery_logs.status = ?", f.Status)
	}
	if f.Source != "" && f.Source != "all" {
		q = q.Where("delivery_logs.source = ?", f.Source)
	}
	if s := f.Search; s != "" {
		like := "%" + s + "%"
		q = q.Where("delivery_logs.title LIKE ? OR delivery_logs.content LIKE ?", like, like)
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

	type row struct {
		models.DeliveryLog
		ReminderTitle   string `gorm:"column:reminder_title"`
		ReminderDeleted bool   `gorm:"column:reminder_deleted"`
	}
	var rows []row
	if err := q.Order("delivery_logs.id DESC").Limit(f.Limit).Offset(f.Offset).Scan(&rows).Error; err != nil {
		return nil, 0, err
	}

	views := make([]*LogView, 0, len(rows))
	for i := range rows {
		views = append(views, &LogView{
			DeliveryLog:     rows[i].DeliveryLog,
			ReminderTitle:   rows[i].ReminderTitle,
			ReminderDeleted: rows[i].ReminderDeleted,
		})
	}
	return views, total, nil
}

// GetDetail 返回单条日志 + attempts + 提醒元数据。
func (s *LogService) GetDetail(id uint) (*LogView, error) {
	var dl models.DeliveryLog
	if err := s.DB.First(&dl, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, middleware.NewAppError(middleware.CodeNotFound, "日志不存在")
		}
		return nil, err
	}

	// 取 reminder（可能已软删）
	var r models.Reminder
	reminderTitle := dl.Title
	reminderDeleted := false
	if err := s.DB.Unscoped().First(&r, dl.ReminderID).Error; err == nil {
		reminderTitle = r.Title
		reminderDeleted = r.DeletedAt.Valid
	}

	// 取 attempts
	var attempts []models.DeliveryAttempt
	s.DB.Where("delivery_log_id = ?", id).Order("attempt ASC").Find(&attempts)

	// 取 confirm_url
	var confirmURL string
	if dl.ConfirmChainID != nil && *dl.ConfirmChainID != "" {
		var tok models.ConfirmToken
		if err := s.DB.Model(&models.ConfirmToken{}).
			Joins("JOIN delivery_logs ON delivery_logs.id = confirm_tokens.delivery_log_id").
			Where("delivery_logs.confirm_chain_id = ?", *dl.ConfirmChainID).
			First(&tok).Error; err == nil {
			confirmURL = s.confirmURL(tok.Token)
		}
	}

	return &LogView{
		DeliveryLog:     dl,
		ReminderTitle:   reminderTitle,
		ReminderDeleted: reminderDeleted,
		Attempts:        attempts,
		ConfirmURL:      confirmURL,
	}, nil
}

// Purge 清理日志。
//
// olderThan > 0 时删除早于 now-olderThan 的日志；
// all=true 时删除全部日志（忽略 olderThan）。
// 返回清理的条数。
//
// 额外清理：
//   - 删除过期/孤立的 confirm_tokens
//   - all=true 时执行 VACUUM（回收 SQLite 空间）
func (s *LogService) Purge(olderThan time.Duration, all bool) (int64, error) {
	q := s.DB.Model(&models.DeliveryLog{})
	if !all && olderThan > 0 {
		cutoff := time.Now().Add(-olderThan)
		q = q.Where("fired_at < ?", cutoff)
	} else if !all {
		return 0, nil
	}

	var ids []uint
	if err := q.Pluck("id", &ids).Error; err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}

	var count int64
	err := s.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("delivery_log_id IN ?", ids).Delete(&models.DeliveryAttempt{}).Error; err != nil {
			return err
		}
		res := tx.Where("id IN ?", ids).Delete(&models.DeliveryLog{})
		if res.Error != nil {
			return res.Error
		}
		count = res.RowsAffected

		// 清理孤立的 confirm_tokens（关联的 delivery_log 已被删）
		if err := tx.Where("delivery_log_id NOT IN (SELECT id FROM delivery_logs)").
			Delete(&models.ConfirmToken{}).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return 0, err
	}

	// all=true 时回收 SQLite 空间
	if all {
		s.DB.Exec("VACUUM")
	}
	return count, nil
}

// PurgeCount 返回即将清理的日志条数（用于二次确认展示）。
func (s *LogService) PurgeCount(olderThan time.Duration, all bool) (int64, error) {
	q := s.DB.Model(&models.DeliveryLog{})
	if !all && olderThan > 0 {
		cutoff := time.Now().Add(-olderThan)
		q = q.Where("fired_at < ?", cutoff)
	} else if !all {
		return 0, nil
	}
	var count int64
	if err := q.Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// confirmURL 拼接确认链接。
func (s *LogService) confirmURL(token string) string {
	return s.PublicURL + "/c/" + token
}