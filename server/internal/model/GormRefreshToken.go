package model

import (
	"time"

	"github.com/google/uuid"
)

type GormRefreshToken struct {
	//UUIDv7
	ID uuid.UUID `gorm:"primarykey;not null;default:gen_random_uuid()"`

	//UUIDv4
	UserID uuid.UUID `gorm:"type:uuid;index:idx_user_revoked;not null"`

	TokenHash string `gorm:"type:varchar(64);uniqueIndex;not null"`

	FamilyID  uuid.UUID `gorm:"type:uuid;index;not null"`
	IsRevoked bool      `gorm:"default:false;index:idx_user_revoked;not null"`

	LastUsedAt time.Time `gorm:"default:null"`

	CsrfTokenHash string `gorm:"not null"`
	DpopJKT       string `gorm:"type:varchar(45);index;not null"`

	SessionID uuid.UUID `gorm:"type:uuid;index;not null"`
	Session   Session   `gorm:"foreignKey:SessionID"`

	ExpiresAt time.Time `gorm:"index"` //żeby kasować szybko stare tokeny
	CreatedAt time.Time
}

// override the table name
func (GormRefreshToken) TableName() string {
	return "refresh_tokens"
}
