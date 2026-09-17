package model

import (
	"time"

	"github.com/google/uuid"
)

type ShareMember struct {
	ShareID uuid.UUID `gorm:"type:uuid;primaryKey;not null"`
	UserID  uuid.UUID `gorm:"type:uuid;primaryKey;index;not null"`

	Permissions int16 `gorm:"type:smallint;not null"`

	EncryptedSharePassphrase       string `gorm:"type:text;not null"`
	SignedEncryptedSharePassphrase string `gorm:"type:text;not null"`

	CreatedAt time.Time
	UpdatedAt time.Time

	Share *Share `gorm:"foreignKey:ShareID"`
	User  *User  `gorm:"foreignKey:UserID"`
}
