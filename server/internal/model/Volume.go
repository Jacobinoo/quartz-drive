package model

import (
	"time"

	"github.com/google/uuid"
)

type VolumeType string

const (
	VolumeTypePrivate      VolumeType = "PRIVATE"
	VolumeTypeOrganization VolumeType = "ORGANIZATION"
)

type Volume struct {
	ID   uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Type VolumeType `gorm:"type:varchar(20);not null"`

	// Relationships
	OwnerUserID *uuid.UUID `gorm:"type:uuid;uniqueIndex:idx_user_private_vol,where:type='PRIVATE'"` // Set if Type == PRIVATE
	RootNodeID  uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex"`

	// Quota Accounting
	StorageQuota int64 `gorm:"not null;default:104857600"`
	StorageUsed  int64 `gorm:"not null;default:0"`

	CreatedAt time.Time
	UpdatedAt time.Time
}
