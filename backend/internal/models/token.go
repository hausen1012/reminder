package models

import "time"

// Token 用于外部程序调用 /api/external/v1/* 时鉴权。
//
// KeyHash 是明文 sha256，Prefix 是明文前 8 位用于列表展示。
// Plaintext 保存完整明文，用于面板按需查看。
type Token struct {
	ID         uint       `gorm:"primaryKey" json:"id"`
	Name       string     `gorm:"size:64;not null" json:"name"`
	KeyHash    string     `gorm:"size:64;uniqueIndex" json:"-"`
	Prefix     string     `gorm:"size:16" json:"prefix"`
	Plaintext  string     `gorm:"type:text" json:"-"`
	Enabled    bool       `gorm:"default:true" json:"enabled"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

// TokenDefaultChannel 是 Token 与默认通道的多对多关联。
// 当外部 API 创建提醒时未指定 channel_ids，则使用 Token 的默认通道集合。
type TokenDefaultChannel struct {
	TokenID   uint `gorm:"primaryKey" json:"token_id"`
	ChannelID uint `gorm:"primaryKey" json:"channel_id"`
}