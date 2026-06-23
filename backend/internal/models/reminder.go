package models

import (
	"time"

	"gorm.io/datatypes"
)

// Reminder 表示一条提醒。
//
// Calendar 与 ScheduleType 共同决定 ScheduleSpec 的 JSON 结构，详见设计文档 §3.2。
// 合法组合：
//   - solar + once / interval / cron
//   - lunar + once / interval
type Reminder struct {
	ID      uint   `gorm:"primaryKey" json:"id"`
	Title   string `gorm:"size:200;not null" json:"title"`
	Content       string `gorm:"type:text" json:"content"`
	ContentFormat string `gorm:"size:8;default:text" json:"content_format"`

	Calendar     string         `gorm:"size:8;index" json:"calendar"`
	ScheduleType string         `gorm:"size:16;index" json:"schedule_type"`
	ScheduleSpec datatypes.JSON `gorm:"type:text" json:"schedule_spec"`
	Timezone     string         `gorm:"size:48" json:"timezone"`

	Enabled  bool   `gorm:"index;default:true" json:"enabled"`
	Source   string `gorm:"size:16;index" json:"source"` // web | api
	APIKeyID *uint  `gorm:"index" json:"api_key_id,omitempty"`

	RequireConfirm          bool `gorm:"default:false" json:"require_confirm"`
	ConfirmRetryIntervalSec int  `gorm:"default:0" json:"confirm_retry_interval_sec"`
	ConfirmMaxRetries       int  `gorm:"default:0" json:"confirm_max_retries"`

	NextFireAt  *time.Time `gorm:"index" json:"next_fire_at,omitempty"` // 始终为公历 UTC
	LastFiredAt *time.Time `json:"last_fired_at,omitempty"`
	FireCount   int        `json:"fire_count"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ReminderChannel 提醒与通道的多对多关联。
type ReminderChannel struct {
	ReminderID uint `gorm:"primaryKey" json:"reminder_id"`
	ChannelID  uint `gorm:"primaryKey" json:"channel_id"`
}
