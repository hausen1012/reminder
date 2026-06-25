// log_service 提供日志查询与清理功能。
//
// 日志是调度触发与 Dispatch 运行的结果沉淀，不可修改仅可查询和按时间范围删除。
package services

import (
	"errors"
	"sort"
	"time"

	"github.com/reminder/backend/internal/middleware"
	"github.com/reminder/backend/internal/models"
	"gorm.io/gorm"
)

// LogService 负责 DeliveryLog 的查询与清理。
type LogService struct {
	DB          *gorm.DB
	PublicURL   string // 用于拼接 confirm_url
}

// NewLogService 构造日志服务。
func NewLogService(db *gorm.DB, publicURL ...string) *LogService {
	url := "http://localhost:8765"
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
	Since      *time.Time // 只返回 fired_at >= 该时间的日志
	Limit      int
	Offset     int
}

// LogView 是返回前端的日志视图（含关联的 remind 基础信息）。
type LogView struct {
	models.DeliveryLog
	ReminderTitle string `json:"reminder_title"`

	// 本日志关联的 attempts（仅详情接口加载）
	Attempts []models.DeliveryAttempt `json:"attempts,omitempty"`

	// 确认链接 URL（仅需要确认且有 token 的日志）
	ConfirmURL string `json:"confirm_url,omitempty"`
}

// List 查询日志列表，返回视图切片与总数。
func (s *LogService) List(f LogFilter) ([]*LogView, int64, error) {
	buildQuery := func() *gorm.DB {
		q := s.DB.Model(&models.DeliveryLog{}).
			Select("delivery_logs.*, COALESCE(r.title, delivery_logs.title) AS reminder_title").
			Joins("LEFT JOIN reminders r ON r.id = delivery_logs.reminder_id")

		if f.ReminderID > 0 {
			q = q.Where("delivery_logs.reminder_id = ?", f.ReminderID)
		}
		if f.Since != nil {
			q = q.Where("delivery_logs.fired_at >= ?", *f.Since)
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
		return q
	}

	if f.Limit <= 0 {
		f.Limit = 50
	}
	if f.Limit > 200 {
		f.Limit = 200
	}

	type row struct {
		models.DeliveryLog
		ReminderTitle string `gorm:"column:reminder_title"`
	}

	var total int64
	if err := buildQuery().Where("delivery_logs.retry_round = 0").Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var mainRows []row
	if err := buildQuery().Where("delivery_logs.retry_round = 0").Order("delivery_logs.id DESC").Limit(f.Limit).Offset(f.Offset).Scan(&mainRows).Error; err != nil {
		return nil, 0, err
	}
	if len(mainRows) == 0 {
		return []*LogView{}, total, nil
	}

	mainByID := make(map[uint]row, len(mainRows))
	ids := make([]uint, 0, len(mainRows))
	chainIDs := make([]string, 0, len(mainRows))
	for _, r := range mainRows {
		mainByID[r.ID] = r
		ids = append(ids, r.ID)
		if r.ConfirmChainID != nil && *r.ConfirmChainID != "" {
			chainIDs = append(chainIDs, *r.ConfirmChainID)
		}
	}

	var subRows []row
	if len(chainIDs) > 0 {
		if err := buildQuery().Where("delivery_logs.retry_round > 0").Where("delivery_logs.confirm_chain_id IN ?", chainIDs).Order("delivery_logs.id ASC").Scan(&subRows).Error; err != nil {
			return nil, 0, err
		}
	}

	groupedSubs := make(map[string][]row, len(chainIDs))
	for _, r := range subRows {
		if r.ConfirmChainID == nil || *r.ConfirmChainID == "" {
			continue
		}
		key := *r.ConfirmChainID
		groupedSubs[key] = append(groupedSubs[key], r)
	}
	for key := range groupedSubs {
		sort.Slice(groupedSubs[key], func(i, j int) bool {
			if groupedSubs[key][i].RetryRound == groupedSubs[key][j].RetryRound {
				return groupedSubs[key][i].ID < groupedSubs[key][j].ID
			}
			return groupedSubs[key][i].RetryRound < groupedSubs[key][j].RetryRound
		})
	}

	views := make([]*LogView, 0, len(mainRows)+len(subRows))
	for _, id := range ids {
		main := mainByID[id]
		views = append(views, &LogView{
			DeliveryLog:     main.DeliveryLog,
			ReminderTitle:   main.ReminderTitle,
		})
		if main.ConfirmChainID == nil || *main.ConfirmChainID == "" {
			continue
		}
		for _, sub := range groupedSubs[*main.ConfirmChainID] {
			views = append(views, &LogView{
				DeliveryLog:     sub.DeliveryLog,
				ReminderTitle:   sub.ReminderTitle,
			})
		}
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

	// 取 reminder（可能已被硬删，此时回退到日志快照标题）
	var r models.Reminder
	reminderTitle := dl.Title
	if err := s.DB.First(&r, dl.ReminderID).Error; err == nil {
		reminderTitle = r.Title
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
		Attempts:        attempts,
		ConfirmURL:      confirmURL,
	}, nil
}

// BatchDelete 按 ID 批量删除日志及关联的投递尝试。
func (s *LogService) BatchDelete(ids []uint) error {
	if len(ids) == 0 {
		return nil
	}
	return s.DB.Transaction(func(tx *gorm.DB) error {
		for _, batch := range chunkIDs(ids, 500) {
			if err := tx.Where("delivery_log_id IN ?", batch).Delete(&models.DeliveryAttempt{}).Error; err != nil {
				return err
			}
			if err := tx.Where("id IN ?", batch).Delete(&models.DeliveryLog{}).Error; err != nil {
				return err
			}
		}
		// 清理孤立的 confirm_tokens
		return tx.Where("delivery_log_id NOT IN (SELECT id FROM delivery_logs)").
			Delete(&models.ConfirmToken{}).Error
	})
}

// Purge 清理日志。
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
for _, batch := range chunkIDs(ids, 500) {
			if err := tx.Where("delivery_log_id IN ?", batch).Delete(&models.DeliveryAttempt{}).Error; err != nil {
				return err
			}
			res := tx.Where("id IN ?", batch).Delete(&models.DeliveryLog{})
			if res.Error != nil {
				return res.Error
			}
			count += res.RowsAffected
		}

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