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
	Email              string       `gorm:"not null;uniqueIndex"`
	RegistrationRecord string       `gorm:"not null"`
	RegistrationNonce  string       `gorm:"not null"`

	KdfParams         dto.KdfParams `gorm:"embedded;embeddedPrefix:kdf_"`
	EncryptionVersion int16         `gorm:"type:smallint;not null"`

	KeyStore UserKeyStore `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE"`

	Shares []Share `gorm:"foreignKey:OwnerID;constraint:OnDelete:CASCADE"`

	ShareMemberships []ShareMember `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE"`
}
