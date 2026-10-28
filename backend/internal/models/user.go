package models

// User 是后台管理员账号。
type User struct {
	ID        uint   `gorm:"primaryKey" json:"id"`
	Username  string `gorm:"uniqueIndex;size:64" json:"username"`
	Password  string `gorm:"size:256" json:"-"`
	CreatedAt Time   `json:"created_at"`
	UpdatedAt Time   `json:"updated_at"`
}
