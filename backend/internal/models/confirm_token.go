package models

import "time"

// ConfirmToken 是确认链接 /c/:token 使用的一次性令牌。
//
// 同一确认重发链上的多轮触发复用同一个 token，用户任何一次点击都标记为已确认并终止整条链。
type ConfirmToken struct {
	Token         string     `gorm:"size:64;primaryKey" json:"token"`
	DeliveryLogID uint       `gorm:"index" json:"delivery_log_id"`
	ExpiresAt     time.Time  `gorm:"index" json:"expires_at"`
	UsedAt        *time.Time `json:"used_at,omitempty"`
}
