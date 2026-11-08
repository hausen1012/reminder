package models

import (
	"gorm.io/datatypes"
)

// Reminder 表示一条提醒。
//
// Calendar + ScheduleType 决定 ScheduleSpec 结构：solar 支持 once/interval/cron，lunar 支持 once/interval。
// 时区是站点级全局设置，不按提醒保存；时间字段统一用 models.Time（Unix 毫秒）。
type Reminder struct {
	ID            uint   `gorm:"primaryKey" json:"id"`
	Title         string `gorm:"size:200;not null" json:"title"`
	Content       string `gorm:"type:text" json:"content"`
	ContentFormat string `gorm:"size:8;default:text" json:"content_format"`

	Calendar     string         `gorm:"size:8;index" json:"calendar"`
	ScheduleType string         `gorm:"size:16;index" json:"schedule_type"`
	ScheduleSpec datatypes.JSON `gorm:"type:text" json:"schedule_spec"`

	// 不能写 gorm:"default:true"：GORM 会跳过带默认值的零值字段，导致 Enabled=false 被覆盖成 true。
	// 默认启用由 ReminderService.Create 显式赋值。Enabled 与 next_fire_at 组成 idx_reminders_due
	// 复合索引，供 sweeper 与 Upcoming 的范围扫描使用。
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
