package models

// AIAction 是 AI 提出的一次写操作，等待用户确认后才执行。
//
// 对话本身不落库（助手是浮动窗里的即时工具，不保存聊天记录），
// 但草稿必须落库：它是「确认后才生效」这条安全机制本身，
// 刷新页面、切页面、甚至关掉浏览器之后，待确认草稿都还在。
//
// PayloadEnc 是服务层入参 JSON 用 secretbox 加密后的 base64 文本：
// create_channel 的 config 里可能带 SMTP 密码、Webhook Authorization 等明文，
// 落库前必须先加密（与通道敏感字段同一套密钥）。
// 应用时先解密、反序列化，再走既有的校验与落库逻辑，不绕过任何约束。
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
