package models

// ConfirmToken 是确认链接 /c/:token 使用的一次性令牌，同一重发链复用同一令牌。
type ConfirmToken struct {
	Token         string `gorm:"size:64;primaryKey" json:"token"`
	DeliveryLogID uint   `gorm:"index" json:"delivery_log_id"`
	ExpiresAt     Time   `gorm:"index" json:"expires_at"`
	UsedAt        *Time  `json:"used_at,omitempty"`
}
