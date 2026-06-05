package models

import (
	"time"

	"gorm.io/datatypes"
)

// Channel 表示一个通知通道。
//
// Type 在创建后不可变更，避免 Config schema 漂移。
// Config 内带 _enc 后缀的字段由 ChannelService 在落库前/读出后透明加解密。
type Channel struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	Name      string         `gorm:"size:64;not null;uniqueIndex" json:"name"`
	Type      string         `gorm:"size:16;index" json:"type"` // smtp | dingtalk | wecom | webhook
	Enabled   bool           `gorm:"default:true" json:"enabled"`
	Config    datatypes.JSON `gorm:"type:text" json:"config"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}
