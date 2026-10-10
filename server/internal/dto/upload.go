package dto

import (
	"time"

	"github.com/google/uuid"
)

type InitFileUploadRequest struct {
	TotalFileSize int64 `json:"totalFileSize" validate:"required,min=1"`
	TotalChunks   int64 `json:"totalChunks" validate:"required,min=1"`

	ParentNodeID                  string `json:"parentNodeId" validate:"required,uuid"`                   // The UUID of the folder where this file is placed
	EncryptedName                 string `json:"encryptedName" validate:"required,base64rawurl,max=1500"` // Ciphertext of filename (e.g. "vacation.jpg")
	NameNonce                     string `json:"nameNonce" validate:"required,len=32"`
	EncryptedNodePassphrase       string `json:"encryptedNodePassphrase" validate:"required,len=1590"` // The fileKey sealed/wrapped for the parent folder!
	SignedEncryptedNodePassphrase string `json:"signedEncryptedNodePassphrase" validate:"required,len=86"`
	NodePublicKey                 string `json:"nodePublicKey" validate:"required,base64rawurl,len=1622"`
	WrappedNodeKey                string `json:"wrappedNodeKey" validate:"required,base64rawurl,len=64"`
	NodePrivNonce                 string `json:"nodePrivNonce" validate:"required,base64rawurl,len=32"`
	HasChildren                   bool   `json:"hasChildren"`

	EncryptedMetadata string `json:"encryptedMetadata" validate:"required,base64rawurl,max=5000"`
	MetadataNonce     string `json:"metadataNonce" validate:"required,base64rawurl,len=32"`
}

type RequestChunkUploadRequest struct {
	UploadID     uuid.UUID `json:"uploadId" validate:"required,uuid"`
	ChunkIndex   int       `json:"chunkIndex" validate:"min=0"`
	DeclaredSize int64     `json:"declaredSize" validate:"required,min=1,max=4194304"` //post-encryption chunk size
	ChunkHash    string    `json:"chunkHash" validate:"required,base64,len=44"`        // SHA-256 of ciphertext for integrity check
}

type RequestChunkUploadResponse struct {
	URL string `json:"url"`
}

type FinishChunkUploadRequest struct {
	UploadID   uuid.UUID `json:"uploadId" validate:"required,uuid"`
	ChunkIndex int       `json:"chunkIndex" validate:"min=0"`
	Etag       string    `json:"etag" validate:"required,max=100"`
	ChunkHash  string    `json:"chunkHash" validate:"required,base64,len=44"`
}

type FinishFileUploadRequest struct {
	NodeID                        string    `json:"nodeId" validate:"required,uuid"`       // The UUID generated for this file node
	ParentNodeID                  string    `json:"parentNodeId" validate:"required,uuid"` // The UUID of the folder where this file is placed
	SizeBytes                     int64     `json:"sizeBytes" validate:"required,min=1"`
	TotalChunks                   int       `json:"totalChunks" validate:"required,min=1"`
	EncryptedName                 string    `json:"encryptedName" validate:"required,base64rawurl,max=1500"` // Ciphertext of filename (e.g. "vacation.jpg")
	NameNonce                     string    `json:"nameNonce" validate:"required,len=32"`
	EncryptedNodePassphrase       string    `json:"encryptedNodePassphrase" validate:"required,len=1590"` // The fileKey sealed/wrapped for the parent folder!
	SignedEncryptedNodePassphrase string    `json:"signedEncryptedNodePassphrase" validate:"required,len=86"`
	NodePublicKey                 string    `json:"nodePublicKey" validate:"required,len=1622"`
	WrappedNodeKey                string    `json:"wrappedNodeKey" validate:"required,len=64"`
	NodePrivNonce                 string    `json:"nodePrivNonce" validate:"required,len=32"`
	HasChildren                   bool      `json:"hasChildren"`
	CreatedAt                     time.Time `json:"createdAt" validate:"required"`
	ChunkNonces                   []string  `json:"chunkNonces" validate:"required,min=1,dive,base64rawurl,len=32"` // Array of nonces used for each 4MB chunk
	ChunkSizes                    []int     `json:"chunkSizes" validate:"required,min=1,dive,min=1"`                // Array of sizes in bytes for each chunk

	EncryptedMetadata string `json:"encryptedMetadata" validate:"required,base64rawurl,max=5000"`
	MetadataNonce     string `json:"metadataNonce" validate:"required,base64rawurl,len=32"`
}

type CreateFolderRequest struct {
	Node struct {
		NodePublicKey  string `json:"nodePublicKey" validate:"required,len=1622"`
		WrappedNodeKey string `json:"wrappedNodeKey" validate:"required,len=64"`
		NodePrivNonce  string `json:"nodePrivNonce" validate:"required,len=32"`
		Signature      string `json:"signature" validate:"required,len=86"`
	} `json:"node"`
	Link struct {
		ParentNodeID                  uuid.UUID `json:"parentNodeId" validate:"required,uuid"`
		EncryptedName                 string    `json:"encryptedName" validate:"required,base64rawurl,max=1500"`
		NameNonce                     string    `json:"nameNonce" validate:"required,len=32"`
		EncryptedNodePassphrase       string    `json:"encryptedNodePassphrase" validate:"required,len=1590"`
		SignedEncryptedNodePassphrase string    `json:"signedEncryptedNodePassphrase" validate:"required,len=86"`
		AuthorID                      uuid.UUID `json:"authorId" validate:"required,uuid"`
	} `json:"link"`
}

type RenameRequest struct {
	EncryptedName string `json:"encryptedName" validate:"required,base64rawurl,max=1500"` //allows up to ~255 characters of raw filename
	NameNonce     string `json:"nameNonce" validate:"required,len=32"`
}

type MoveFileRequest struct {
	NodeID            string `json:"nodeId" validate:"required,uuid"`
	OldParentFolderID string `json:"oldParentFolderId" validate:"required,uuid"`
	NewParentFolderID string `json:"newParentFolderId" validate:"required,uuid"`

	// newly wrapped payloads for the Destination Folder
	NewEncryptedName             string `json:"newEncryptedName" validate:"required,base64rawurl,max=1500"`
	NewNameNonce                 string `json:"newNameNonce" validate:"required,len=32"`
	NewEncryptedNodePassphrase   string `json:"newEncryptedNodePassphrase" validate:"required,len=1590"`
	NewSignedEncryptedPassphrase string `json:"newSignedEncryptedNodePassphrase" validate:"required,len=86"`
}
