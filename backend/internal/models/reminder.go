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
//
// 时区不在提醒上：站点时区是全局设置（设置页可改），由 config 表 key=timezone 保存。
// 所有时间字段以 UTC 落库——SQLite 按带偏移的文本比较时间，混合偏移会错序。
type Reminder struct {
	ID            uint   `gorm:"primaryKey" json:"id"`
	Title         string `gorm:"size:200;not null" json:"title"`
	Content       string `gorm:"type:text" json:"content"`
	ContentFormat string `gorm:"size:8;default:text" json:"content_format"`

	Calendar     string         `gorm:"size:8;index" json:"calendar"`
	ScheduleType string         `gorm:"size:16;index" json:"schedule_type"`
	ScheduleSpec datatypes.JSON `gorm:"type:text" json:"schedule_spec"`

	// 注意：不能写 gorm:"default:true"。GORM 在 Create 时会跳过带默认值的零值字段，
	// 导致 Enabled=false 被数据库默认值覆盖成 true（"创建即禁用"会静默失效）。
	// 默认启用由业务代码（ReminderService.Create）显式赋值保证。
	// 也不加 not null：历史库若存在 enabled 为 NULL 的行，AutoMigrate 重建表会因
	// NOT NULL 约束失败而启动不了；本字段的修复只需要去掉 default，不需要加约束。
	Enabled bool   `gorm:"index" json:"enabled"`
	Source  string `gorm:"size:16;index" json:"source"` // web | api
	TokenID *uint  `gorm:"index" json:"token_id,omitempty"`

	RequireConfirm          bool `gorm:"default:false" json:"require_confirm"`
	ConfirmRetryIntervalSec int  `gorm:"default:0" json:"confirm_retry_interval_sec"`
	ConfirmMaxRetries       int  `gorm:"default:0" json:"confirm_max_retries"`

	NextFireAt  *time.Time `gorm:"index" json:"next_fire_at,omitempty"` // 始终为公历 UTC
	LastFiredAt *time.Time `json:"last_fired_at,omitempty"`             // 始终为公历 UTC
	FireCount   int        `json:"fire_count"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ReminderChannel 提醒与通道的多对多关联。
type ReminderChannel struct {
	ReminderID uint `gorm:"primaryKey" json:"reminder_id"`
	ChannelID  uint `gorm:"primaryKey" json:"channel_id"`
}
