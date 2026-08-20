// Package secrets 提供敏感字段(API key、MQTT 口令、bot 私钥)落盘前的
// AES-256-GCM 加密与读取时的透明解密。
//
// 用法:启动时以环境变量 MESH_SECRET_KEY 初始化(SetSecretKey)。
// 未配置密钥时 Encrypt/Decrypt 均原样透传(兼容旧部署的明文数据);
// 加密值带 "enc:v1:" 前缀,解密时对无前缀的历史明文直接返回。
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"sync"
)

const prefix = "enc:v1:"

var (
	mu  sync.RWMutex
	key []byte
)

// SetSecretKey 用 MESH_SECRET_KEY 的内容派生 AES-256 密钥;传空则回到明文模式。
func SetSecretKey(secret string) {
	mu.Lock()
	defer mu.Unlock()
	secret = strings.TrimSpace(secret)
	if secret == "" {
		key = nil
		return
	}
	sum := sha256.Sum256([]byte(secret))
	key = sum[:]
}

// Enabled 报告是否已配置加密密钥。
func Enabled() bool {
	mu.RLock()
	defer mu.RUnlock()
	return len(key) > 0
}

// Encrypt 加密明文;未配置密钥时原样返回。
func Encrypt(plain string) string {
	if plain == "" {
		return plain
	}
	mu.RLock()
	k := key
	mu.RUnlock()
	if len(k) == 0 {
		return plain
	}
	block, err := aes.NewCipher(k)
	if err != nil {
		return plain
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return plain
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return plain
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plain), nil)
	return prefix + base64.RawStdEncoding.EncodeToString(sealed)
}

// Decrypt 解密 enc:v1: 前缀的值;无前缀(历史明文)或解密失败返回空串。
func Decrypt(value string) string {
	if !strings.HasPrefix(value, prefix) {
		return value
	}
	mu.RLock()
	k := key
	mu.RUnlock()
	if len(k) == 0 {
		return value
	}
	raw, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(value, prefix))
	if err != nil {
		return ""
	}
	block, err := aes.NewCipher(k)
	if err != nil {
		return ""
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return ""
	}
	if len(raw) < gcm.NonceSize() {
		return ""
	}
	nonce, ciphertext := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return ""
	}
	return string(plain)
}
