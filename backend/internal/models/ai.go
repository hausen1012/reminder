package models

// AIAction 记录 AI 生成的待确认写操作，PayloadEnc 为加密存储的参数 JSON。
type AIAction struct {
	ID         string `gorm:"primaryKey;size:64" json:"id"`
	Kind       string `gorm:"size:32;not null" json:"kind"` // create_channel | create_reminder | update_reminder | delete_reminder
	Summary    string `gorm:"size:255" json:"summary"`
	PayloadEnc string `gorm:"type:text" json:"-"`
	Status     string `gorm:"size:16;index" json:"status"` // pending | applied | rejected | failed
	Error      string `gorm:"size:255" json:"error,omitempty"`
	ResultRef  string `gorm:"size:64" json:"result_ref,omitempty"`
	CreatedAt  Time   `json:"created_at"`
	UpdatedAt  Time   `json:"updated_at"`
}
