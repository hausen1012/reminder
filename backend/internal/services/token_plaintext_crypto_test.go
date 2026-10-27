package services

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"github.com/reminder/backend/internal/crypto/secretbox"
	"github.com/reminder/backend/internal/middleware"
	"github.com/reminder/backend/internal/models"
)

// 令牌明文必须以密文入库，不能出现可直接使用的 bdrk_ 明文。
func TestTokenPlaintextStoredEncrypted(t *testing.T) {
	svc, db := newTokenSvc(t)

	plain, key, err := svc.Create("加密存储", nil)
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	var stored models.Token
	if err := db.First(&stored, key.ID).Error; err != nil {
		t.Fatalf("读取令牌失败: %v", err)
	}
	if stored.Plaintext == plain {
		t.Fatalf("明文被直接入库: %q", stored.Plaintext)
	}
	if strings.HasPrefix(stored.Plaintext, keyPrefix) {
		t.Fatalf("库里仍能看到令牌前缀，说明是明文存储: %q", stored.Plaintext)
	}
	if stored.Plaintext == "" {
		t.Fatal("明文列不应为空")
	}

	// 但仍然能取回原文
	got, err := svc.GetPlaintext(key.ID)
	if err != nil {
		t.Fatalf("解密失败: %v", err)
	}
	if got != plain {
		t.Fatalf("解密结果 = %q，期望 %q", got, plain)
	}

	// 鉴权用的哈希不受影响
	if _, ok := svc.Verify(plain); !ok {
		t.Fatal("加密存储后明文仍应能通过校验")
	}
}

// 早期版本的令牌是明文入库，升级后应仍能查看（不能因为解不开就报错）。
func TestTokenPlaintextLegacyCompat(t *testing.T) {
	svc, db := newTokenSvc(t)

	const legacy = "bdrk_legacyplaintextkey000000000000"
	if err := db.Create(&models.Token{
		Name:      "历史令牌",
		KeyHash:   sha256Hex(legacy),
		Prefix:    legacy[:len(keyPrefix)+8],
		Plaintext: legacy, // 老数据：直接存明文
		Enabled:   true,
	}).Error; err != nil {
		t.Fatalf("写入历史数据失败: %v", err)
	}

	var key models.Token
	if err := db.First(&key, "key_hash = ?", sha256Hex(legacy)).Error; err != nil {
		t.Fatalf("查询失败: %v", err)
	}

	got, err := svc.GetPlaintext(key.ID)
	if err != nil {
		t.Fatalf("历史明文应可读取，得到错误: %v", err)
	}
	if got != legacy {
		t.Fatalf("历史明文 = %q，期望 %q", got, legacy)
	}
}

// 没有配置 Box（测试或早期路径）时应原样存取，不应崩。
func TestTokenPlaintextWithoutBox(t *testing.T) {
	db := openTestDB(t)
	svc := NewTokenService(db, nil)

	plain, key, err := svc.Create("无密钥", nil)
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	got, err := svc.GetPlaintext(key.ID)
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if got != plain {
		t.Fatalf("明文 = %q，期望 %q", got, plain)
	}
}

// 换密钥后解不开时应返回 404 + 可操作提示，而不是 500，也不能把密文当明文返回。
func TestTokenPlaintextWithWrongKeyReturnsNotFound(t *testing.T) {
	db := openTestDB(t)

	boxA, err := secretbox.New("")
	if err != nil {
		t.Fatalf("构造 secretbox 失败: %v", err)
	}
	plain, key, err := NewTokenService(db, boxA).Create("换密钥", nil)
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	// 换用另一把密钥构造服务，模拟 ENCRYPTION_KEY 被改动
	other := make([]byte, 32)
	for i := range other {
		other[i] = byte(i + 1)
	}
	boxB, err := secretbox.New(base64.StdEncoding.EncodeToString(other))
	if err != nil {
		t.Fatalf("构造第二把密钥失败: %v", err)
	}

	got, err := NewTokenService(db, boxB).GetPlaintext(key.ID)
	if err == nil {
		t.Fatalf("解不开时应报错，实际返回 %q", got)
	}

	// 必须是 not_found（404），不能是 internal_error（500）
	var ae *middleware.AppError
	if !errors.As(err, &ae) {
		t.Fatalf("错误类型 = %T，期望 *middleware.AppError", err)
	}
	if ae.Code != middleware.CodeNotFound {
		t.Fatalf("错误码 = %q，期望 %q（不能是 500）", ae.Code, middleware.CodeNotFound)
	}
	if ae.Message == "" {
		t.Fatal("应给出可操作的提示文案")
	}

	// 关键：不能把库里的密文当明文返回，否则用户会误以为那是令牌
	if got == plain {
		t.Fatal("换密钥后不应还能拿到原明文")
	}
	if got != "" {
		t.Fatalf("出错时不应返回任何内容，实际 %q", got)
	}

	// 令牌本身照旧可用：鉴权走独立保存的 sha256 哈希
	if _, ok := NewTokenService(db, boxB).Verify(plain); !ok {
		t.Fatal("换密钥不应影响令牌鉴权")
	}
}
