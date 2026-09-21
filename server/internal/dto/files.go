package dto

import "time"

type FileListResponseItem struct {
	NodeID                        string `json:"nodeId"`
	ParentNodeID                  string `json:"parentNodeId"`
	Type                          string `json:"type"` // 'FILE' or 'FOLDER'
	SizeBytes                     int64  `json:"sizeBytes"`
	EncryptedName                 string `json:"encryptedName"`
	NameNonce                     string `json:"nameNonce"`
	EncryptedNodePassphrase       string `json:"encryptedNodePassphrase"`
	SignedEncryptedNodePassphrase string `json:"signedEncryptedNodePassphrase"`

	NodePublicKey  string `json:"nodePublicKey"`
	WrappedNodeKey string `json:"wrappedNodeKey"`
	NodePrivNonce  string `json:"nodePrivNonce"`

	HasChildren bool `json:"hasChildren"`

	CreatedAt time.Time `json:"createdAt"`

	EncryptedMetadata string `json:"encryptedMetadata"`
	MetadataNonce     string `json:"metadataNonce"`

	AuthorEmail            string `json:"authorEmail"`
	AuthorSigningPublicKey string `json:"authorSigningPublicKey"`
}

type DownloadUrlsRequest struct {
	NodeID       string `json:"nodeId"`
	ChunkIndices []int  `json:"chunkIndices;omitempty"`
}

type DownloadUrlResponseItem struct {
	Index     int    `json:"index"`
	URL       string `json:"url"`
	SizeBytes int64  `json:"sizeBytes"`
}
