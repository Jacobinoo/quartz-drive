package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ChunkUploadStatus string

const (
	ChunkUploadStatusPending  ChunkUploadStatus = "PENDING"
	ChunkUploadStatusUploaded ChunkUploadStatus = "UPLOADED"
	ChunkUploadStatusVerified ChunkUploadStatus = "VERIFIED"
	ChunkUploadStatusFailed   ChunkUploadStatus = "FAILED"
	ChunkUploadStatusFlagged  ChunkUploadStatus = "FLAGGED"
)

type UploadChunk struct {
	gorm.Model
	ID         uuid.UUID `gorm:"primarykey;not null;default:uuidv7()"`
	UploadID   uuid.UUID `gorm:"index;type:uuid;not null"`
	ChunkIndex int64     `gorm:"index;not null"`
	//"authors/X/uploads/X/nodes/X/chunk_n"
	ObjectKey string `gorm:"not null"`
	// ExpectedSize int64             `gorm:"not null"`
	Status        ChunkUploadStatus `gorm:"type:varchar(50);check:status IN ('PENDING', 'UPLOADED','VERIFIED','FAILED','FLAGGED');not null"`
	GeneratedUrls int               `gorm:"not null;default:0"`
	Etag          string            `gorm:"default:null"`
	DeclaredSize  int64             `gorm:"default:null"`

	VerifiedSize int64     `gorm:"default:null"`
	VerifiedAt   time.Time `gorm:"default:null"`

	Upload Upload `gorm:"foreignKey:UploadID"`
}
