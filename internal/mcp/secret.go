package mcp

import (
	"crypto/aes"
	stdcipher "crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
)

// credentialPrefix 标记已加密的凭据，解密失败时可识别出脏数据。
const credentialPrefix = "enc:v1:"

// Cipher 用 AES-256-GCM 加解密上游凭据（密钥由配置派生）。
// 未配置密钥时 Enabled 为 false，凭据按明文原样存取（仅开发环境）。
type Cipher struct {
	aead stdcipher.AEAD
}

// NewCipher 用任意长度密钥派生 32 字节 AES 密钥。
func NewCipher(key string) (*Cipher, error) {
	if strings.TrimSpace(key) == "" {
		return &Cipher{}, nil
	}

	sum := sha256.Sum256([]byte(key))
	block, err := aes.NewCipher(sum[:])
	if err != nil {
		return nil, err
	}
	aead, err := stdcipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	return &Cipher{aead: aead}, nil
}

// Enabled 表示是否真正加密。
func (c *Cipher) Enabled() bool {
	return c != nil && c.aead != nil
}

// Encrypt 加密凭据；空串原样返回。
func (c *Cipher) Encrypt(plain string) (string, error) {
	if plain == "" || !c.Enabled() {
		return plain, nil
	}

	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := c.aead.Seal(nil, nonce, []byte(plain), nil)

	return credentialPrefix + base64.RawURLEncoding.EncodeToString(append(nonce, sealed...)), nil
}

// Decrypt 解密凭据；明文（无前缀）原样返回，便于平滑迁移。
func (c *Cipher) Decrypt(stored string) (string, error) {
	if stored == "" {
		return "", nil
	}
	if !strings.HasPrefix(stored, credentialPrefix) {
		return stored, nil
	}
	if !c.Enabled() {
		return "", errors.New("mcp: credential_key is not configured")
	}

	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(stored, credentialPrefix))
	if err != nil {
		return "", err
	}
	nonceSize := c.aead.NonceSize()
	if len(raw) <= nonceSize {
		return "", errors.New("mcp: malformed credential")
	}
	plain, err := c.aead.Open(nil, raw[:nonceSize], raw[nonceSize:], nil)
	if err != nil {
		return "", err
	}

	return string(plain), nil
}
