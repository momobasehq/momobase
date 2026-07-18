package platform

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

// Encryptor encrypts and authenticates application secrets with AES-256-GCM.
type Encryptor struct{ key []byte }

// NewEncryptor decodes a base64-encoded 32-byte encryption key.
func NewEncryptor(encoded string) (*Encryptor, error) {
	key, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode encryption key: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("encryption key must decode to exactly 32 bytes, got %d", len(key))
	}
	return &Encryptor{key}, nil
}
func (e *Encryptor) aead() (cipher.AEAD, error) {
	block, err := aes.NewCipher(e.key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// Encrypt seals plain with a random nonce and returns base64-encoded ciphertext.
func (e *Encryptor) Encrypt(plain []byte) (string, error) {
	gcm, err := e.aead()
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(gcm.Seal(nonce, nonce, plain, nil)), nil
}

// Decrypt authenticates and opens base64-encoded ciphertext produced by Encrypt.
func (e *Encryptor) Decrypt(encoded string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}
	gcm, err := e.aead()
	if err != nil {
		return nil, err
	}
	if len(raw) < gcm.NonceSize() {
		return nil, fmt.Errorf("ciphertext too short")
	}
	return gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
}
