package models

import (
	"gorm.io/datatypes"
)

// Channel 表示通知通道，Config 中 _enc 后缀字段透明加解密存储。
type Channel struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	Name      string         `gorm:"size:64;not null;uniqueIndex" json:"name"`
	Type      string         `gorm:"size:16;index" json:"type"`
	Config    datatypes.JSON `gorm:"type:text" json:"config"`
	CreatedAt Time           `json:"created_at"`
	UpdatedAt Time           `json:"updated_at"`
}
