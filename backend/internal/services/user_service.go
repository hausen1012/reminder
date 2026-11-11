// user_service 提供用户认证、查询与密码修改服务。
package services

import (
	"errors"

	"github.com/reminder/backend/internal/middleware"
	"github.com/reminder/backend/internal/models"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// UserService 提供用户相关的业务能力。
type UserService struct {
	DB *gorm.DB
}

// NewUserService 构造用户服务。
func NewUserService(db *gorm.DB) *UserService {
	return &UserService{DB: db}
}

// Authenticate 校验用户名与密码，失败时返回统一错误以防枚举。
func (s *UserService) Authenticate(username, password string) (*models.User, error) {
	var user models.User
	if err := s.DB.Where("username = ?", username).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, middleware.NewAppError(middleware.CodeUnauthorized, "用户名或密码错误")
		}
		return nil, err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return nil, middleware.NewAppError(middleware.CodeUnauthorized, "用户名或密码错误")
	}
	return &user, nil
}

// Get 按 ID 返回用户。
func (s *UserService) Get(id uint) (*models.User, error) {
	var user models.User
	if err := s.DB.First(&user, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, middleware.NewAppError(middleware.CodeNotFound, "用户不存在")
		}
		return nil, err
	}
	return &user, nil
}

// ChangePassword 校验原密码后更新为新密码。
func (s *UserService) ChangePassword(id uint, oldPassword, newPassword string) error {
	user, err := s.Get(id)
	if err != nil {
		return err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(oldPassword)); err != nil {
		return middleware.NewAppError(middleware.CodeValidationFailed, "原密码错误").WithField("old_password")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return s.DB.Model(&models.User{}).Where("id = ?", id).
		Update("password", string(hash)).Error
}
