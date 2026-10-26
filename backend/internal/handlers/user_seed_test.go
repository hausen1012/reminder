package handlers

import (
	"testing"

	"github.com/reminder/backend/internal/models"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// seedUser 创建一个用户名为 admin、密码为 admin123 的测试用户。
func seedUser(t *testing.T, db *gorm.DB) *models.User {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("生成密码哈希失败: %v", err)
	}
	user := &models.User{Username: "admin", Password: string(hash)}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("创建测试用户失败: %v", err)
	}
	return user
}
