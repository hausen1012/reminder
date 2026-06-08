package services

import (
	"testing"

	"github.com/bedrock/backend/internal/crypto/secretbox"
	"github.com/bedrock/backend/internal/models"
)

func TestChannelUpdateRejectsMissingSMTPFieldsAfterMerge(t *testing.T) {
	db := openTestDB(t)
	box, err := secretbox.New("")
	if err != nil {
		t.Fatalf("构造 secretbox 失败: %v", err)
	}
	service := NewChannelService(db, box)

	created, err := service.Create(ChannelInput{
		Name: "smtp-test",
		Type: "smtp",
		Config: map[string]any{
			"host": "smtp.example.com",
			"port": 587,
			"username": "user@example.com",
			"password_enc": "secret",
			"from_addr": "sender@example.com",
			"from_name": "提醒助手",
			"to": []string{"receiver@example.com"},
			"use_starttls": true,
		},
	})
	if err != nil {
		t.Fatalf("创建通道失败: %v", err)
	}

	_, err = service.Update(created.ID, ChannelInput{
		Name: created.Name,
		Config: map[string]any{
			"host": "",
		},
	})
	if err == nil {
		t.Fatal("期望更新时拦截空 host")
	}

	_, err = service.Update(created.ID, ChannelInput{
		Name: created.Name,
		Config: map[string]any{
			"to": []string{},
		},
	})
	if err == nil {
		t.Fatal("期望更新时拦截空收件人")
	}

	_, err = service.Update(created.ID, ChannelInput{
		Name: created.Name,
		Config: map[string]any{
			"port": 0,
		},
	})
	if err == nil {
		t.Fatal("期望更新时拦截非法端口")
	}
}

func TestChannelUpdateKeepsEncryptedPasswordWhenPlaceholderIsUsed(t *testing.T) {
	db := openTestDB(t)
	box, err := secretbox.New("")
	if err != nil {
		t.Fatalf("构造 secretbox 失败: %v", err)
	}
	service := NewChannelService(db, box)

	created, err := service.Create(ChannelInput{
		Name: "smtp-secret",
		Type: "smtp",
		Config: map[string]any{
			"host": "smtp.example.com",
			"port": 587,
			"username": "user@example.com",
			"password_enc": "secret",
			"from_addr": "sender@example.com",
			"from_name": "提醒助手",
			"to": []string{"receiver@example.com"},
			"use_starttls": true,
		},
	})
	if err != nil {
		t.Fatalf("创建通道失败: %v", err)
	}

	var before models.Channel
	if err := db.First(&before, created.ID).Error; err != nil {
		t.Fatalf("读取更新前通道失败: %v", err)
	}
	plainBefore, err := service.DecryptedConfig(&before)
	if err != nil {
		t.Fatalf("解密更新前配置失败: %v", err)
	}

	_, err = service.Update(created.ID, ChannelInput{
		Name: "smtp-secret-updated",
		Config: map[string]any{
			"password_enc": "***",
			"from_name": "新提醒助手",
		},
	})
	if err != nil {
		t.Fatalf("使用占位符更新失败: %v", err)
	}

	var after models.Channel
	if err := db.First(&after, created.ID).Error; err != nil {
		t.Fatalf("读取更新后通道失败: %v", err)
	}
	plainAfter, err := service.DecryptedConfig(&after)
	if err != nil {
		t.Fatalf("解密更新后配置失败: %v", err)
	}

	if string(plainBefore) == "" || string(plainAfter) == "" {
		t.Fatal("明文配置不应为空")
	}
	if string(plainBefore) == string(after.Config) {
		t.Fatal("落库配置不应是明文")
	}
	if string(plainAfter) == string(after.Config) {
		t.Fatal("更新后落库配置不应是明文")
	}
	if string(plainBefore) == string(plainAfter) {
		// from_name 已修改，整体 JSON 应变化
	} else {
		// 继续验证 password 保持不变
	}

	cfgBefore, err := service.decryptConfig(before.Config)
	if err != nil {
		t.Fatalf("解析更新前配置失败: %v", err)
	}
	cfgAfter, err := service.decryptConfig(after.Config)
	if err != nil {
		t.Fatalf("解析更新后配置失败: %v", err)
	}
	if cfgBefore["password_enc"] != cfgAfter["password_enc"] {
		t.Fatalf("密码占位符更新后应保留原值，before=%v after=%v", cfgBefore["password_enc"], cfgAfter["password_enc"])
	}
	if cfgAfter["from_name"] != "新提醒助手" {
		t.Fatalf("from_name 未更新，实际 %v", cfgAfter["from_name"])
	}
}
