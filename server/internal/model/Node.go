package model

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type NodeType string

const (
	NodeTypeFile   NodeType = "FILE"
	NodeTypeFolder NodeType = "FOLDER"
)

type Node struct {
	gorm.Model
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;not null"`
	Type      NodeType  `gorm:"type:varchar(20);check:type IN ('FILE', 'FOLDER');not null"`
	SizeBytes int64     `gorm:"default:0;not null"`

	EncryptedMetadata string `gorm:"type:text"`
	MetadataNonce     string `gorm:"type:text"`

	OwnerID  uuid.UUID `gorm:"type:uuid;not null;index"`
	VolumeID uuid.UUID `gorm:"type:uuid;not null;index"`

	NodePublicKey  string `gorm:"type:text;not null"`
	WrappedNodeKey string `gorm:"type:text;not null"`
	NodePrivNonce  string `gorm:"type:text;not null"`

	Signature string `gorm:"type:text;not null"`

	// 1. Jeśli to FOLDER:
	// Folder zawiera wiele Linków (wpisów w katalogu).
	// Relacja: Folder (ParentNode) -> Linki (Children).
	ChildrenLinks []Link `gorm:"foreignKey:ParentNodeID"`

	// 2. Jeśli to PLIK:
	// Plik składa się z bloków danych (Chunks).
	// Relacja: Plik (Node) -> Bloki.
	FileBlocks []FileBlock `gorm:"foreignKey:NodeID"`
}
