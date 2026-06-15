package models

import "time"

// Config 站点设置 key-value 存储（类似 WordPress options）。
// Key 为主键，Value 为自由文本，可存 SVG、JSON 等任意内容。
// 新增配置无需改表结构，直接插入新行即可。
type Config struct {
	Key       string    `gorm:"primaryKey;size:128" json:"key"`
	Value     string    `gorm:"type:text" json:"value"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
