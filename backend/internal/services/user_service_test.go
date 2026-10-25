package services

import (
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/reminder/backend/internal/models"
)

// 正确的用户名密码应通过认证。
func TestUserService_Authenticate_OK(t *testing.T) {
	db := openTestDB(t)
	hash, _ := bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.MinCost)
	if err := db.Create(&models.User{Username: "admin", Password: string(hash)}).Error; err != nil {
		t.Fatalf("创建用户失败: %v", err)
	}

	user, err := NewUserService(db).Authenticate("admin", "admin123")
	if err != nil {
		t.Fatalf("认证失败: %v", err)
	}
	if user.Username != "admin" {
		t.Fatalf("username = %q", user.Username)
	}
}

// 密码错误与用户不存在返回同一类错误，避免泄露用户名是否存在。
func TestUserService_Authenticate_Rejects(t *testing.T) {
	db := openTestDB(t)
	hash, _ := bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.MinCost)
	db.Create(&models.User{Username: "admin", Password: string(hash)})

	svc := NewUserService(db)
	if _, err := svc.Authenticate("admin", "wrong"); err == nil {
		t.Fatal("期望密码错误被拒绝")
	}
	if _, err := svc.Authenticate("nobody", "admin123"); err == nil {
		t.Fatal("期望不存在的用户被拒绝")
	}
}

// 修改密码：原密码错误拒绝，正确则新密码可登录、旧密码失效。
func TestUserService_ChangePassword(t *testing.T) {
	db := openTestDB(t)
	hash, _ := bcrypt.GenerateFromPassword([]byte("old123"), bcrypt.MinCost)
	user := &models.User{Username: "admin", Password: string(hash)}
	db.Create(user)

	svc := NewUserService(db)
	if err := svc.ChangePassword(user.ID, "wrong", "new123"); err == nil {
		t.Fatal("期望原密码错误被拒绝")
	}
	if err := svc.ChangePassword(user.ID, "old123", "new123"); err != nil {
		t.Fatalf("修改密码失败: %v", err)
	}

	if _, err := svc.Authenticate("admin", "old123"); err == nil {
		t.Fatal("旧密码应已失效")
	}
	if _, err := svc.Authenticate("admin", "new123"); err != nil {
		t.Fatalf("新密码应可登录: %v", err)
	}
}
