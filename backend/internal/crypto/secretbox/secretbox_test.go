package secretbox

import (
	"encoding/base64"
	"testing"
)

func TestRoundTripFallback(t *testing.T) {
	b, err := New("")
	if err != nil {
		t.Fatalf("使用 fallback 构造 Box 失败: %v", err)
	}
	plain := "smtp-password-12345"
	ct, err := b.EncryptString(plain)
	if err != nil {
		t.Fatalf("加密失败: %v", err)
	}
	if ct == plain {
		t.Fatal("加密结果不能与明文相同")
	}
	got, err := b.DecryptString(ct)
	if err != nil {
		t.Fatalf("解密失败: %v", err)
	}
	if got != plain {
		t.Fatalf("解密结果错误：want %q, got %q", plain, got)
	}
}

func TestRoundTripCustomKey(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	b, err := New(base64.StdEncoding.EncodeToString(key))
	if err != nil {
		t.Fatalf("使用自定义 key 构造 Box 失败: %v", err)
	}
	got, err := b.EncryptString("hello")
	if err != nil {
		t.Fatal(err)
	}
	out, err := b.DecryptString(got)
	if err != nil {
		t.Fatal(err)
	}
	if out != "hello" {
		t.Fatalf("want hello, got %q", out)
	}
}

func TestInvalidKey(t *testing.T) {
	if _, err := New("not-base64-!!!"); err == nil {
		t.Fatal("非法 base64 应该报错")
	}
	if _, err := New(base64.StdEncoding.EncodeToString([]byte("too-short"))); err == nil {
		t.Fatal("长度非 32 字节应该报错")
	}
}

func TestEncryptUniqueness(t *testing.T) {
	b, _ := New("")
	a, _ := b.EncryptString("same plaintext")
	c, _ := b.EncryptString("same plaintext")
	if a == c {
		t.Fatal("nonce 应该使每次加密结果不同")
	}
}

func TestDecryptTooShort(t *testing.T) {
	b, _ := New("")
	if _, err := b.Decrypt([]byte("x")); err == nil {
		t.Fatal("过短的密文应该报错")
	}
}
