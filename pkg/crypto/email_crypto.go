package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"quartz/config"
)

// TODO: check for these keys on app startup and exit if they are missing or invalid
func GetEmailEncryptionKey() ([]byte, error) {
	keyStr := config.Cfg.CRYPTO.EmailEncryptionKey
	if keyStr == "" {
		return nil, errors.New("EmailEncryptionKey is missing from config")
	}
	keyBytes, err := base64.RawURLEncoding.DecodeString(keyStr)
	if err != nil {
		// Fallback for standard base64 if needed
		keyBytes, err = base64.StdEncoding.DecodeString(keyStr)
		if err != nil {
			return nil, errors.New("EmailEncryptionKey is invalid base64")
		}
	}
	if len(keyBytes) != 32 {
		return nil, errors.New("EmailEncryptionKey must be exactly 32 bytes for AES-256")
	}
	return keyBytes, nil
}

func GetEmailHashSecretKey() ([]byte, error) {
	keyStr := config.Cfg.CRYPTO.EmailHashSecretKey
	if keyStr == "" {
		return nil, errors.New("EmailHashSecretKey is missing from config")
	}
	keyBytes, err := base64.RawURLEncoding.DecodeString(keyStr)
	if err != nil {
		// Fallback for standard base64 if needed
		keyBytes, err = base64.StdEncoding.DecodeString(keyStr)
		if err != nil {
			return nil, errors.New("EmailHashSecretKey is invalid base64")
		}
	}
	if len(keyBytes) != 32 {
		return nil, errors.New("KV_KEY must be exactly 32 bytes for AES-256")
	}
	return keyBytes, nil
}

func EncryptEmail(plaintext []byte) (string, error) {
	key, err := GetEmailEncryptionKey()
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

func DecryptEmail(encrypted string) ([]byte, error) {
	key, err := GetEmailEncryptionKey()
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

func HashEmail(message []byte) (string, error) {
	key, err := GetEmailHashSecretKey()
	if err != nil {
		return "", err
	}

	h := hmac.New(sha256.New, key)
	h.Write(message)
	return hex.EncodeToString(h.Sum(nil)), nil
}
