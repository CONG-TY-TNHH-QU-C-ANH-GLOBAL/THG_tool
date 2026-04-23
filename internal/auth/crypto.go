package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
)

// deriveKey converts any-length string to a 32-byte AES-256 key via SHA-256.
func deriveKey(key string) []byte {
	h := sha256.Sum256([]byte(key))
	return h[:]
}

// Encrypt encrypts plaintext with AES-256-GCM and returns a base64-encoded ciphertext.
// Returns plaintext unchanged if key is empty (no-op mode for dev environments).
func Encrypt(plaintext, key string) (string, error) {
	if key == "" || plaintext == "" {
		return plaintext, nil
	}
	block, err := aes.NewCipher(deriveKey(key))
	if err != nil {
		return "", err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := aead.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt decrypts a base64-encoded AES-256-GCM ciphertext.
// Falls back to returning the input unchanged for legacy plaintext values.
func Decrypt(ciphertext64, key string) (string, error) {
	if key == "" || ciphertext64 == "" {
		return ciphertext64, nil
	}
	data, err := base64.StdEncoding.DecodeString(ciphertext64)
	if err != nil {
		// Not base64 → treat as legacy plaintext
		return ciphertext64, nil
	}
	block, err := aes.NewCipher(deriveKey(key))
	if err != nil {
		return "", err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	ns := aead.NonceSize()
	if len(data) < ns {
		return "", errors.New("ciphertext too short")
	}
	plaintext, err := aead.Open(nil, data[:ns], data[ns:], nil)
	if err != nil {
		// Decryption failed — could be unencrypted legacy value; return as-is
		return ciphertext64, nil
	}
	return string(plaintext), nil
}
