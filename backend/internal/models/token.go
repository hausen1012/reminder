package models

// Token 用于外部 API 鉴权，Plaintext 密文存储，KeyHash 为 sha256。
type Token struct {
	ID        uint   `gorm:"primaryKey" json:"id"`
	Name      string `gorm:"size:64;not null" json:"name"`
	KeyHash   string `gorm:"size:64;uniqueIndex" json:"-"`
	Prefix    string `gorm:"size:16" json:"prefix"`
	Plaintext string `gorm:"type:text" json:"-"`
	// 不加 default:true：GORM 会跳过带默认值的零值字段，导致 Enabled=false 被覆盖。
	Enabled    bool  `json:"enabled"`
	LastUsedAt *Time `json:"last_used_at,omitempty"`
	CreatedAt  Time  `json:"created_at"`
}

// TokenDefaultChannel 是 Token 与默认通道的多对多关联。
// 当外部 API 创建提醒时未指定 channel_ids，则使用 Token 的默认通道集合。
type TokenDefaultChannel struct {
	TokenID   uint `gorm:"primaryKey" json:"token_id"`
	ChannelID uint `gorm:"primaryKey" json:"channel_id"`
}
