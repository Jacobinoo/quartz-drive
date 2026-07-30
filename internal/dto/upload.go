package dto

import (
	"time"

	"github.com/google/uuid"
)

type InitFileUploadRequest struct {
	NodeID      string `json:"nodeId"`
	TotalChunks int    `json:"totalChunks"`
}

type FinishFileUploadRequest struct {
	NodeID                        string    `json:"nodeId"`       // The UUID generated for this file node
	ParentNodeID                  string    `json:"parentNodeId"` // The UUID of the folder where this file is placed
	SizeBytes                     int64     `json:"sizeBytes"`
	TotalChunks                   int       `json:"totalChunks"`
	EncryptedName                 string    `json:"encryptedName"` // Ciphertext of filename (e.g. "vacation.jpg")
	NameNonce                     string    `json:"nameNonce"`
	EncryptedNodePassphrase       string    `json:"encryptedNodePassphrase"` // The fileKey sealed/wrapped for the parent folder!
	SignedEncryptedNodePassphrase string    `json:"signedEncryptedNodePassphrase"`
	NodePublicKey                 string    `json:"nodePublicKey"`
	WrappedNodeKey                string    `json:"wrappedNodeKey"`
	NodePrivNonce                 string    `json:"nodePrivNonce"`
	HasChildren                   bool      `json:"hasChildren"`
	CreatedAt                     time.Time `json:"createdAt"`
	ChunkNonces                   []string  `json:"chunkNonces"` // Array of nonces used for each 4MB chunk
	ChunkSizes                    []int     `json:"chunkSizes"`  // Array of sizes in bytes for each chunk

	EncryptedMetadata string `json:"encryptedMetadata"`
	MetadataNonce     string `json:"metadataNonce"`
}

type CreateFolderRequest struct {
	Node struct {
		NodePublicKey  string `json:"nodePublicKey"`
		WrappedNodeKey string `json:"wrappedNodeKey"`
		NodePrivNonce  string `json:"nodePrivNonce"`
		Signature      string `json:"signature"`
	} `json:"node"`
	Link struct {
		ParentNodeID                  uuid.UUID `json:"parentNodeId"`
		EncryptedName                 string    `json:"encryptedName"`
		NameNonce                     string    `json:"nameNonce"`
		EncryptedNodePassphrase       string    `json:"encryptedNodePassphrase"`
		SignedEncryptedNodePassphrase string    `json:"signedEncryptedNodePassphrase"`
		AuthorID                      uuid.UUID `json:"authorId"`
	} `json:"link"`
}

type RenameRequest struct {
	EncryptedName string `json:"encryptedName"`
	NameNonce     string `json:"nameNonce"`
}

type MoveFileRequest struct {
	NodeID            string `json:"nodeId"`
	OldParentFolderID string `json:"oldParentFolderId"`
	NewParentFolderID string `json:"newParentFolderId"`

	// newly wrapped payloads for the Destination Folder
	NewEncryptedName             string `json:"newEncryptedName"`
	NewNameNonce                 string `json:"newNameNonce"`
	NewEncryptedNodePassphrase   string `json:"newEncryptedNodePassphrase"`
	NewSignedEncryptedPassphrase string `json:"newSignedEncryptedNodePassphrase"`
}
