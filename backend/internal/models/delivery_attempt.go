package models

// DeliveryAttempt 记录单个通道在一次投递中的尝试详情。
type DeliveryAttempt struct {
	ID            uint   `gorm:"primaryKey" json:"id"`
	DeliveryLogID uint   `gorm:"index" json:"delivery_log_id"`
	ChannelID     uint   `gorm:"index" json:"channel_id"`
	ChannelType   string `gorm:"size:16" json:"channel_type"`
	ChannelName   string `gorm:"size:64" json:"channel_name"`
	Attempt       int    `json:"attempt"`               // 1, 2, 3
	Status        string `gorm:"size:16" json:"status"` // success | failed
	Error         string `gorm:"type:text" json:"error,omitempty"`
	LatencyMs     int    `json:"latency_ms"`
	CreatedAt     Time   `gorm:"index" json:"created_at"`
}
