package model

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ShareType string

const (
	ShareTypeDefault ShareType = "DEFAULT"
	ShareTypeShared  ShareType = "SHARED"
)

type Share struct {
	gorm.Model
	ID           uuid.UUID `gorm:"type:uuid;primaryKey;not null"`
	TargetLinkID uuid.UUID `gorm:"type:uuid;not null;index"`
	Type         ShareType `gorm:"type:varchar(50);check:type IN ('DEFAULT', 'SHARED');not null"`

	OwnerID uuid.UUID `gorm:"type:uuid;not null;index"`

	SharePublicKey         string `gorm:"type:text;not null"`
	WrappedSharePrivateKey string `gorm:"type:text;not null"`
	SharePrivNonce         string `gorm:"type:text;not null"`

	TargetLink *Link         `gorm:"foreignKey:TargetLinkID"`
	Owner      *User         `gorm:"foreignKey:OwnerID"`
	Members    []ShareMember `gorm:"foreignKey:ShareID;constraint:OnDelete:CASCADE"`
}
