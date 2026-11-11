// 调度器内共用的乐观锁推进与日志构造。
package scheduler

import (
	"github.com/reminder/backend/internal/models"
	"gorm.io/gorm"
)

// advanceReminder 使用乐观锁推进 next_fire_at，返回 false 表示已被其他操作推进。
func advanceReminder(q *gorm.DB, r *models.Reminder, nextStored *models.Time, updates map[string]any) (bool, error) {
	if r.NextFireAt != nil {
		q = q.Where("next_fire_at = ?", *r.NextFireAt)
	} else {
		q = q.Where("next_fire_at IS NULL")
	}
	updates["next_fire_at"] = nextStored

	res := q.Updates(updates)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

// newDeliveryLog 构造一次触发的日志快照（标题与正文取当前值）。
func newDeliveryLog(r *models.Reminder, status string, firedAt models.Time, retryRound int) *models.DeliveryLog {
	return &models.DeliveryLog{
		ReminderID: r.ID,
		FiredAt:    firedAt,
		Title:      r.Title,
		Content:    r.Content,
		Status:     status,
		Source:     r.Source,
		RetryRound: retryRound,
	}
}
