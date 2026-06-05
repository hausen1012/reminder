// Package secretbox 提供通道敏感字段（SMTP 密码、钉钉签名密钥、Webhook Authorization 等）的对称加解密。
//
// 密钥来源优先级：
//  1. 环境变量 SECRET_BOX_KEY（base64 编码的 32 字节）；
//  2. 否则使用本文件内置的硬编码 fallback 常量。
//
// 生产部署建议覆盖 SECRET_BOX_KEY 环境变量，否则 DB 文件泄漏即等于明文。
// 启动时不会因为缺少 SECRET_BOX_KEY 而退出。
package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

// fallbackKey 是当环境变量未配置时使用的硬编码 key（32 字节）。
// 这个值会被编译进二进制，安全等级等同于无加密；仅用于"开箱即用"。
var fallbackKey = []byte{
	0x52, 0x65, 0x6d, 0x69, 0x6e, 0x64, 0x65, 0x72,
	0x53, 0x65, 0x63, 0x72, 0x65, 0x74, 0x42, 0x6f,
	0x78, 0x46, 0x61, 0x6c, 0x6c, 0x62, 0x61, 0x63,
	0x6b, 0x4b, 0x65, 0x79, 0x32, 0x30, 0x32, 0x36,
}

// Box 是一个对称加解密器，持有一个 AES-256 key。
type Box struct {
	gcm cipher.AEAD
}

// New 根据 base64 编码的 keyB64 构造 Box。
// keyB64 为空字符串时使用内置 fallbackKey。
// keyB64 不为空但解析失败 / 长度不是 32 字节时返回错误。
func New(keyB64 string) (*Box, error) {
	var key []byte
	if keyB64 == "" {
		key = fallbackKey
	} else {
		raw, err := base64.StdEncoding.DecodeString(keyB64)
		if err != nil {
			return nil, fmt.Errorf("SECRET_BOX_KEY 不是合法 base64: %w", err)
		}
		if len(raw) != 32 {
			return nil, fmt.Errorf("SECRET_BOX_KEY 解码后必须是 32 字节，实际 %d", len(raw))
		}
		key = raw
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("构造 AES cipher 失败: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("构造 GCM 失败: %w", err)
	}
	return &Box{gcm: gcm}, nil
}

// Encrypt 加密明文，返回 nonce||ciphertext 的拼接结果。
func (b *Box) Encrypt(plain []byte) ([]byte, error) {
	nonce := make([]byte, b.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("生成 nonce 失败: %w", err)
	}
	return b.gcm.Seal(nonce, nonce, plain, nil), nil
}

// Decrypt 解密 Encrypt 产生的密文。
func (b *Box) Decrypt(cipherBytes []byte) ([]byte, error) {
	ns := b.gcm.NonceSize()
	if len(cipherBytes) < ns {
		return nil, errors.New("密文长度小于 nonce 大小")
	}
	nonce, ct := cipherBytes[:ns], cipherBytes[ns:]
	return b.gcm.Open(nil, nonce, ct, nil)
}

// EncryptString 是 Encrypt 的字符串便捷封装，返回 base64 编码结果。
func (b *Box) EncryptString(plain string) (string, error) {
	out, err := b.Encrypt([]byte(plain))
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(out), nil
}

// DecryptString 是 Decrypt 的字符串便捷封装，输入 base64。
func (b *Box) DecryptString(cipherB64 string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(cipherB64)
	if err != nil {
		return "", fmt.Errorf("密文不是合法 base64: %w", err)
	}
	out, err := b.Decrypt(raw)
	if err != nil {
		return "", err
	}
	return string(out), nil
}
