package model

import (
	"time"

	"github.com/google/uuid"
)

type Session struct {
	ID                 uuid.UUID `gorm:"primarykey;not null;default:uuidv7()"`
	UserID             uuid.UUID `gorm:"type:uuid;index;not null"`
	DevicePublicKey    string    `gorm:"type:text;"`
	WrappedAccountKeys string    `gorm:"type:text;"`

	// Optional metadata for a "Manage Devices" UI:
	UserAgent    string `gorm:"type:text"`
	DeviceName   string `gorm:"type:text"`
	LastActiveAt time.Time
	CreatedAt    time.Time
}
