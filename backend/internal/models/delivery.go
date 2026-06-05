package models

import "time"

// DeliveryLog 是一次提醒触发的总记录。
//
// 同一确认重发链上的多条记录共享同一 ConfirmChainID，便于前端折叠展示。
type DeliveryLog struct {
	ID             uint       `gorm:"primaryKey" json:"id"`
	ReminderID     uint       `gorm:"index" json:"reminder_id"`
	FiredAt        time.Time  `gorm:"index" json:"fired_at"`
	Title          string     `gorm:"size:200" json:"title"`        // 触发瞬间标题快照
	Content        string     `gorm:"type:text" json:"content"`     // 渲染后内容快照
	Status         string     `gorm:"size:16;index" json:"status"`  // pending | success | partial | failed | expired
	Confirmed      bool       `gorm:"index;default:false" json:"confirmed"`
	ConfirmedAt    *time.Time `json:"confirmed_at,omitempty"`
	ConfirmChainID *string    `gorm:"size:36;index" json:"confirm_chain_id,omitempty"`
	RetryRound     int        `json:"retry_round"` // 0 = 第一次触发，>0 = 第 N 次重发
	Source         string     `gorm:"size:16;index" json:"source"`
	CreatedAt      time.Time  `json:"created_at"`
}
