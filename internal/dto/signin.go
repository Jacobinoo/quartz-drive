package dto

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type LoginTrustAttestationHeader struct {
	Status      string `json:"status"`
	Attestation bool   `json:"attestation"`
}

type LoginTrustAttestationConfirmedBody struct {
	LoginTrustAttestationHeader
	TrustedUserInformation
	Token     string `json:"token"`
	CsrfToken string `json:"csrfToken"`
}

type TrustedUserInformation struct {
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deletedAt"`

	ID    uuid.UUID `gorm:"primarykey;type:uuid" json:"id"`
	Email string    `json:"email"`

	KdfParams         KdfParams `gorm:"embedded;embeddedPrefix:kdf_" json:"kdfParams"`
	EncryptionVersion int16     `json:"encryptionVersion"`

	//joined UserKeyStore
	MasterKdfSalt string `json:"masterKdfSalt"`

	AccountEncryptionPublicKey           string `json:"accountEncryptionPublicKey"`
	EncryptedAccountEncryptionPrivateKey string `json:"encryptedAccountEncryptionPrivateKey"`
	AccountEncryptionKeyNonce            string `json:"accountEncryptionKeyNonce"`

	AccountSigningPublicKey           string `json:"accountSigningPublicKey"`
	EncryptedAccountSigningPrivateKey string `json:"encryptedAccountSigningPrivateKey"`
	AccountSigningKeyNonce            string `json:"accountSigningKeyNonce"`
}

// override the table name
func (TrustedUserInformation) TableName() string {
	return "users"
}

type M1Login struct {
	Email        string `json:"email"`
	LoginRequest string `json:"loginRequest"`
	Token        string `json:"token"`
}

type M3Login struct {
	FinishLoginRequest string `json:"finishLoginRequest"`
	Nonce              string
}

type RegisterDeviceRequest struct {
	DevicePublicKey    string `json:"devicePublicKey"`
	WrappedAccountKeys string `json:"wrappedAccountKeys"`
}

type M2Login struct {
	Status        string `json:"status"`
	LoginResponse string `json:"loginResponse"`
	Nonce         string `json:"nonce"`
}

type KdfParams struct {
	KdfAlg      int8  `json:"kdfAlg"`
	KdfOpsLimit int8  `json:"kdfOpsLimit"`
	KdfMemLimit int64 `json:"kdfMemLimit"`
}

func (k KdfParams) Value() (driver.Value, error) {
	// Serialize the KdfParams struct into a format suitable for storage
	// For example, you might serialize it into a JSON string
	paramsJSON, err := json.Marshal(k)
	if err != nil {
		return nil, err
	}
	return string(paramsJSON), nil
}

func (k *KdfParams) Scan(value interface{}) error {
	// Deserialize the value from the database into the KdfParams struct
	// For example, if the value is stored as a JSON string, you would unmarshal it
	paramsJSON, ok := value.(string)
	if !ok {
		return errors.New("unexpected type for kdf params")
	}
	return json.Unmarshal([]byte(paramsJSON), k)
}

func (k ApakeDTO) Value() (driver.Value, error) {
	paramsJSON, err := json.Marshal(k)
	if err != nil {
		return nil, err
	}
	return string(paramsJSON), nil
}

func (k *ApakeDTO) Scan(value interface{}) error {
	paramsJSON, ok := value.(string)
	if !ok {
		return errors.New("unexpected type for apake")
	}
	return json.Unmarshal([]byte(paramsJSON), k)
}
