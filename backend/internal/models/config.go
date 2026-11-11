package models

// Config 为站点键值配置存储。
type Config struct {
	Key       string `gorm:"primaryKey;size:128" json:"key"`
	Value     string `gorm:"type:text" json:"value"`
	CreatedAt Time   `json:"created_at"`
	UpdatedAt Time   `json:"updated_at"`
}
