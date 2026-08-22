package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"quartz/config"
)

func getKVKey() ([]byte, error) {
	keyStr := config.Cfg.KV.Key
	if keyStr == "" {
		return nil, errors.New("KV_KEY is missing from config")
	}
	keyBytes, err := base64.RawURLEncoding.DecodeString(keyStr)
	if err != nil {
		// Fallback for standard base64 if needed
		keyBytes, err = base64.StdEncoding.DecodeString(keyStr)
		if err != nil {
			return nil, errors.New("KV_KEY is invalid base64")
		}
	}
	if len(keyBytes) != 32 {
		return nil, errors.New("KV_KEY must be exactly 32 bytes for AES-256")
	}
	return keyBytes, nil
}

func EncryptRedisPayload(plaintext []byte) (string, error) {
	key, err := getKVKey()
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, aesGCM.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	// For AES-GCM, Seal appends the authentication tag to the ciphertext.
	// Since we are prepending the nonce to the ciphertext, the final structure is:
	// [ nonce | ciphertext | tag ]
	ciphertext := aesGCM.Seal(nonce, nonce, plaintext, nil)
	return base64.RawURLEncoding.EncodeToString(ciphertext), nil
}

func DecryptRedisPayload(encrypted string) ([]byte, error) {
	key, err := getKVKey()
	if err != nil {
		return nil, err
	}

	encBytes, err := base64.RawURLEncoding.DecodeString(encrypted)
	if err != nil {
		// fallback to std encoding just in case
		encBytes, err = base64.StdEncoding.DecodeString(encrypted)
		if err != nil {
			return nil, err
		}
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := aesGCM.NonceSize()
	if len(encBytes) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}

	nonce, ciphertext := encBytes[:nonceSize], encBytes[nonceSize:]
	plaintext, err := aesGCM.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}
