package model

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
)

type UserKeyStore struct {
	UserID uuid.UUID `gorm:"type:uuid;primaryKey;not null"`

	MasterKdfSalt string `gorm:"type:text;not null"`

	AccountEncryptionPublicKey           string `gorm:"type:text;not null"`
	EncryptedAccountEncryptionPrivateKey string `gorm:"type:text;not null"`
	AccountEncryptionKeyNonce            string `gorm:"type:text;not null"`

	AccountSigningPublicKey           string `gorm:"type:text;not null"`
	EncryptedAccountSigningPrivateKey string `gorm:"type:text;not null"`
	AccountSigningKeyNonce            string `gorm:"type:text;not null"`

	RecoveryEncryptedAccountEncryptionPrivateKey string `gorm:"type:text;not null"`
	RecoveryAccountEncryptionKeyNonce            string `gorm:"type:text;not null"`
	RecoveryEncryptedAccountSigningPrivateKey    string `gorm:"type:text;not null"`
	RecoveryAccountSigningKeyNonce               string `gorm:"type:text;not null"`

	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt sql.NullTime `gorm:"index"`
}
