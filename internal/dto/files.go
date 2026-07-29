package dto

import "time"

type FileListResponseItem struct {
	NodeID                        string `json:"nodeId"`
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
}
