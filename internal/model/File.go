package model

import "gorm.io/gorm"

type File struct {
	gorm.Model
	UserID           uint
	Filename         string // base64 encrypted
	MimeType         string // optional
	Size             int64
	Ciphertext       string // base64 combined chunks
	Nonce            string // optional
	EncryptedFileKey string
	Header           string // secretstream header
}
