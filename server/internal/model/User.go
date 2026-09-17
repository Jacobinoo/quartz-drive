package model

import (
	"database/sql"
	"quartz/internal/dto"
	"time"

	"github.com/google/uuid"
)

type User struct {
	CreatedAt          time.Time
	UpdatedAt          time.Time
	DeletedAt          sql.NullTime `gorm:"index"`
	ID                 uuid.UUID    `gorm:"primaryKey;type:uuid;not null"`
	EncryptedEmail     string       `gorm:"not null;"`
	HashedEmail        string       `gorm:"not null;uniqueIndex"`
	RegistrationRecord string       `gorm:"not null"`
	HashedRecoveryID   *string      `gorm:"uniqueIndex;default:null"`

	KdfParams         dto.KdfParams `gorm:"embedded;embeddedPrefix:kdf_"`
	EncryptionVersion int16         `gorm:"type:smallint;default:-1;not null"`

	KeyStore UserKeyStore `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE"`

	Shares []Share `gorm:"foreignKey:OwnerID;constraint:OnDelete:CASCADE"`

	ShareMemberships []ShareMember `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE"`

	StorageQuota int64 `gorm:"not null;default:104857600"` //104857600 = 100MiB
	StorageUsed  int64 `gorm:"not null;default:0"`
}
