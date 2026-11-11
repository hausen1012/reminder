package models

import (
	"gorm.io/datatypes"
)

// Reminder 表示一条提醒，时间字段统一为 Unix 毫秒（models.Time）。
type Reminder struct {
	ID            uint   `gorm:"primaryKey" json:"id"`
	Title         string `gorm:"size:200;not null" json:"title"`
	Content       string `gorm:"type:text" json:"content"`
	ContentFormat string `gorm:"size:8;default:text" json:"content_format"`

	Calendar     string         `gorm:"size:8;index" json:"calendar"`
	ScheduleType string         `gorm:"size:16;index" json:"schedule_type"`
	ScheduleSpec datatypes.JSON `gorm:"type:text" json:"schedule_spec"`

	// 默认值由创建逻辑显式赋值；与 next_fire_at 建立复合索引 idx_reminders_due。
	Enabled bool   `gorm:"index;index:idx_reminders_due,priority:1" json:"enabled"`
	Source  string `gorm:"size:16;index" json:"source"` // web | api
	TokenID *uint  `gorm:"index" json:"token_id,omitempty"`

	RequireConfirm          bool `gorm:"default:false" json:"require_confirm"`
	ConfirmRetryIntervalSec int  `gorm:"default:0" json:"confirm_retry_interval_sec"`
	ConfirmMaxRetries       int  `gorm:"default:0" json:"confirm_max_retries"`

	NextFireAt  *Time `gorm:"index;index:idx_reminders_due,priority:2" json:"next_fire_at,omitempty"` // 始终为公历 UTC
	LastFiredAt *Time `json:"last_fired_at,omitempty"`                                                // 始终为公历 UTC
	FireCount   int   `json:"fire_count"`

	CreatedAt Time `json:"created_at"`
	UpdatedAt Time `json:"updated_at"`
}

// ReminderChannel 提醒与通道的多对多关联。
type ReminderChannel struct {
	ReminderID uint `gorm:"primaryKey" json:"reminder_id"`
	ChannelID  uint `gorm:"primaryKey" json:"channel_id"`
}
