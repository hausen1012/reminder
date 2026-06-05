package models

import "time"

// DeliveryAttempt 记录单个通道在一次 DeliveryLog 中的某一次尝试。
//
// ChannelType / ChannelName 冗余存储，便于通道删除后日志仍能完整展示。
type DeliveryAttempt struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	DeliveryLogID uint      `gorm:"index" json:"delivery_log_id"`
	ChannelID     uint      `gorm:"index" json:"channel_id"`
	ChannelType   string    `gorm:"size:16" json:"channel_type"`
	ChannelName   string    `gorm:"size:64" json:"channel_name"`
	Attempt       int       `json:"attempt"`               // 1, 2, 3
	Status        string    `gorm:"size:16" json:"status"` // success | failed
	Error         string    `gorm:"type:text" json:"error,omitempty"`
	LatencyMs     int       `json:"latency_ms"`
	CreatedAt     time.Time `json:"created_at"`
}
