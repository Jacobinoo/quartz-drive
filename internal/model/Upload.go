package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type UploadStatus string

const (
	UploadStatusPending          UploadStatus = "PENDING"
	UploadStatusComplete         UploadStatus = "COMPLETE"
	UploadStatusFailed           UploadStatus = "FAILED"
	UploadStatusFlaggedMalicious UploadStatus = "FLAGGED_MALICIOUS"
)

type Upload struct {
	gorm.Model
	ID                uuid.UUID    `gorm:"primarykey;not null;default:gen_random_uuid()"`
	UserID            uuid.UUID    `gorm:"type:uuid;not null"`
	NodeID            uuid.UUID    `gorm:"type:uuid;not null"`
	TotalChunks       int64        `gorm:"not null"`
	ReportedTotalSize int64        `gorm:"not null"`
	Status            UploadStatus `gorm:"type:varchar(50);check:status IN ('PENDING', 'COMPLETE','FAILED','FLAGGED_MALICIOUS');not null"`
	ExpiresAt         time.Time    `gorm:"not null"`

	User User `gorm:"foreignKey:UserID"`

	ParentNodeID                  *uuid.UUID `gorm:"type:uuid;index"`
	EncryptedName                 string     `gorm:"type:text;not null;"`
	NameNonce                     string     `gorm:"type:text;not null"`
	EncryptedNodePassphrase       string     `gorm:"type:text;not null"`
	SignedEncryptedNodePassphrase string     `gorm:"type:text;not null"`
	NodePublicKey                 string     `gorm:"type:text;not null"`
	WrappedNodeKey                string     `gorm:"type:text;not null"`
	NodePrivNonce                 string     `gorm:"type:text;not null"`
	HasChildren                   bool       `gorm:"type:bool;not null;default:false"`

	EncryptedMetadata string `gorm:"type:text"`
	MetadataNonce     string `gorm:"type:text"`
	IsUpdate          bool   `gorm:"type:boolean;not null;default:false"`
}
