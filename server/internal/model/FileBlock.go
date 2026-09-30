package model

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type FileBlock struct {
	gorm.Model
	ID uuid.UUID `gorm:"type:uuid;primaryKey;not null"`

	NodeID uuid.UUID `gorm:"type:uuid;not null;index"`
	Node   Node      `gorm:"foreignKey:NodeID;constraint:OnDelete:CASCADE"`

	Index int `gorm:"not null"`

	Bucket string `gorm:"size:64;not null;default:'default'"`

	ObjectKey string `gorm:"size:255;not null;uniqueIndex"`

	// EncryptingNodeID stores the original node ID used in the AEAD Additional Data
	// during encryption. For directly uploaded files this equals NodeID.
	// For cloned/template files this preserves the original template node ID so
	// the decrypt worker can reconstruct the correct AD.
	EncryptingNodeID *uuid.UUID `gorm:"type:uuid"`

	//Header string // secretstream header
	Size int `gorm:"not null"`
}
