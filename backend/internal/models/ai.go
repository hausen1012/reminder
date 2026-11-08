package models

// AIAction 是 AI 提出的一次写操作，等待用户确认后才执行；对话本身不落库。
//
// PayloadEnc 是入参 JSON 加密后的文本（config 里可能带 SMTP 密码等明文），
// 执行时先解密再走既有校验与落库逻辑。
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
