package dto

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
)

type KeysDTO struct {
	MasterKdfSalt string `json:"masterKdfSalt"`

	AccountEncryptionPublicKey           string `json:"accountEncryptionPublicKey"`
	EncryptedAccountEncryptionPrivateKey string `json:"encAccountEncryptionPrivateKey"`
	AccountEncryptionKeyNonce            string `json:"accountEncryptionKeyNonce"`

	AccountSigningPublicKey           string `json:"accountSigningPublicKey"`
	EncryptedAccountSigningPrivateKey string `json:"encAccountSigningPrivateKey"`
	AccountSigningKeyNonce            string `json:"accountSigningKeyNonce"`

	RecoveryEncryptedAccountEncryptionPrivateKey string `json:"recoveryEncAccountEncryptionPrivateKey"`
	RecoveryAccountEncryptionKeyNonce            string `json:"recoveryAccountEncryptionKeyNonce"`
	RecoveryEncryptedAccountSigningPrivateKey    string `json:"recoveryEncAccountSigningPrivateKey"`
	RecoveryAccountSigningKeyNonce               string `json:"recoveryAccountSigningKeyNonce"`
}

func (k KeysDTO) Value() (driver.Value, error) {
	paramsJSON, err := json.Marshal(k)
	if err != nil {
		return nil, err
	}
	return string(paramsJSON), nil
}

func (k *KeysDTO) Scan(value interface{}) error {
	paramsJSON, ok := value.(string)
	if !ok {
		return errors.New("unexpected type for keys")
	}
	return json.Unmarshal([]byte(paramsJSON), k)
}

type UpdateUserKeysRequest struct {
	EncryptedAccountEncryptionPrivateKey string `json:"encAccountEncryptionPrivateKey"`
	AccountEncryptionKeyNonce            string `json:"accountEncryptionKeyNonce"`
	EncryptedAccountSigningPrivateKey    string `json:"encAccountSigningPrivateKey"`
	AccountSigningKeyNonce               string `json:"accountSigningKeyNonce"`

	SignatureHex string `json:"signatureHex"`
}
