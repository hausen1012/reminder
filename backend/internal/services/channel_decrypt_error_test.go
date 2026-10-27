package services

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"

	"github.com/reminder/backend/internal/crypto/secretbox"
	"github.com/reminder/backend/internal/middleware"
	"github.com/reminder/backend/internal/models"
	"gorm.io/datatypes"
)

// 换过 ENCRYPTION_KEY 后，通道敏感字段解不开必须返回 400 + 可操作提示，
// 而不是 500。用户看到「服务器内部错误」完全无法判断要怎么办。
func TestChannelDecryptWrongKeyReturnsValidationError(t *testing.T) {
	db := openTestDB(t)

	boxA, err := secretbox.New("")
	if err != nil {
		t.Fatalf("构造 secretbox 失败: %v", err)
	}
	cipher, err := boxA.EncryptString("smtp-password")
	if err != nil {
		t.Fatalf("加密失败: %v", err)
	}
	cfg, _ := json.Marshal(map[string]any{
		"host":         "smtp.example.com",
		"password_enc": cipher,
	})
	ch := &models.Channel{Name: "换密钥", Type: "smtp", Enabled: true, Config: datatypes.JSON(cfg)}
	if err := db.Create(ch).Error; err != nil {
		t.Fatalf("创建通道失败: %v", err)
	}

	// 用另一把密钥构造服务，模拟 ENCRYPTION_KEY 被改动
	other := make([]byte, 32)
	for i := range other {
		other[i] = byte(i + 1)
	}
	boxB, err := secretbox.New(base64.StdEncoding.EncodeToString(other))
	if err != nil {
		t.Fatalf("构造第二把密钥失败: %v", err)
	}
	svc := NewChannelService(db, boxB, testTZ(cstZone()))

	var stored models.Channel
	if err := db.First(&stored, ch.ID).Error; err != nil {
		t.Fatalf("读取通道失败: %v", err)
	}

	_, err = svc.DecryptedConfig(&stored)
	if err == nil {
		t.Fatal("解不开时应报错")
	}

	var ae *middleware.AppError
	if !errors.As(err, &ae) {
		t.Fatalf("错误类型 = %T，期望 *middleware.AppError（否则会变成 500）", err)
	}
	if ae.Code != middleware.CodeValidationFailed {
		t.Fatalf("错误码 = %q，期望 %q", ae.Code, middleware.CodeValidationFailed)
	}
	// 提示里要说清怎么办
	for _, want := range []string{"ENCRYPTION_KEY", "重新填写"} {
		if !containsSub(ae.Message, want) {
			t.Fatalf("提示文案缺少 %q: %q", want, ae.Message)
		}
	}
	// field 应指向具体字段名（去掉 _enc 后缀）
	if ae.Field != "password" {
		t.Fatalf("field = %q，期望 password", ae.Field)
	}
}

// 正常密钥时解密应照旧可用。
func TestChannelDecryptOKWithSameKey(t *testing.T) {
	db := openTestDB(t)
	svc, _ := newChannelSvcForTest(t)

	cipher, err := svc.Box.EncryptString("smtp-password")
	if err != nil {
		t.Fatalf("加密失败: %v", err)
	}
	cfg, _ := json.Marshal(map[string]any{"password_enc": cipher})
	ch := &models.Channel{Name: "同密钥", Type: "smtp", Enabled: true, Config: datatypes.JSON(cfg)}
	if err := db.Create(ch).Error; err != nil {
		t.Fatalf("创建通道失败: %v", err)
	}

	plain, err := svc.DecryptedConfig(ch)
	if err != nil {
		t.Fatalf("解密失败: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(plain, &out); err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if out["password_enc"] != "smtp-password" {
		t.Fatalf("解密结果 = %v", out["password_enc"])
	}
}

// 试发失败要返回 400，不能是 500。
func TestChannelDryRunFailureIsValidationError(t *testing.T) {
	svc, _ := newChannelSvcForTest(t)

	// 缺少必填字段的 webhook 配置 → 发送器返回永久错误
	err := svc.DryRun(t.Context(), "webhook", map[string]any{})
	if err == nil {
		t.Fatal("缺少 url 的 webhook 试发应失败")
	}
	var ae *middleware.AppError
	if !errors.As(err, &ae) {
		t.Fatalf("错误类型 = %T，期望 *middleware.AppError（否则会变成 500）", err)
	}
	if ae.Code != middleware.CodeValidationFailed {
		t.Fatalf("错误码 = %q，期望 %q", ae.Code, middleware.CodeValidationFailed)
	}
	if !containsSub(ae.Message, "试发失败") {
		t.Fatalf("提示缺少前缀: %q", ae.Message)
	}
}
