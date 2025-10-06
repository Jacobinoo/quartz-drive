package model

import (
	"time"

	"github.com/google/uuid"
)

type GormRefreshToken struct {
	TokenHash  string    `gorm:"primarykey;not null"`
	UserID     uuid.UUID `gorm:"type:uuid;not null"`
	CreatedAt  time.Time
	ExpiresAt  time.Time
	LastUsedAt time.Time `gorm:"default:null"`
	CsrfToken  string    `gorm:"not null"`
}

// override the table name
func (GormRefreshToken) TableName() string {
	return "refresh_tokens"
}
