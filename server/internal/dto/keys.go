package dto

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
)

type KeysDTO struct {
	MasterKdfSalt string `json:"masterKdfSalt" validate:"required,len=22"`

	AccountEncryptionPublicKey           string `json:"accountEncryptionPublicKey" validate:"required,len=1622"`
	EncryptedAccountEncryptionPrivateKey string `json:"encAccountEncryptionPrivateKey" validate:"required,len=64"`
	AccountEncryptionKeyNonce            string `json:"accountEncryptionKeyNonce" validate:"required,len=32"`

	AccountSigningPublicKey           string `json:"accountSigningPublicKey" validate:"required,len=43"`
	EncryptedAccountSigningPrivateKey string `json:"encAccountSigningPrivateKey" validate:"required,len=107"`
	AccountSigningKeyNonce            string `json:"accountSigningKeyNonce" validate:"required,len=32"`

	RecoveryEncryptedAccountEncryptionPrivateKey string `json:"recoveryEncAccountEncryptionPrivateKey" validate:"required,len=64"`
	RecoveryAccountEncryptionKeyNonce            string `json:"recoveryAccountEncryptionKeyNonce" validate:"required,len=32"`
	RecoveryEncryptedAccountSigningPrivateKey    string `json:"recoveryEncAccountSigningPrivateKey" validate:"required,len=107"`
	RecoveryAccountSigningKeyNonce               string `json:"recoveryAccountSigningKeyNonce" validate:"required,len=32"`
	RecoveryIDHex                                string `json:"recoveryIdHex" validate:"omitempty,len=64"`
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
	EncryptedAccountEncryptionPrivateKey string `json:"encAccountEncryptionPrivateKey" validate:"required,len=64"`
	AccountEncryptionKeyNonce            string `json:"accountEncryptionKeyNonce" validate:"required,len=32"`
	EncryptedAccountSigningPrivateKey    string `json:"encAccountSigningPrivateKey" validate:"required,len=107"`
	AccountSigningKeyNonce               string `json:"accountSigningKeyNonce" validate:"required,len=32"`

	SignatureHex string `json:"signatureHex" validate:"required,len=128"`
}
