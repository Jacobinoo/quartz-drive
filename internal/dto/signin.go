package dto

import (
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
	ID                  uuid.UUID      `gorm:"primarykey;type:uuid" json:"id"`
	Email               string         `json:"email"`
	Salt                string         `json:"salt"`
	PublicKey           string         `json:"publicKey"`
	Nonce               string         `json:"nonce"`
	EncryptedPrivateKey string         `json:"encryptedPrivateKey"`
	CreatedAt           time.Time      `json:"createdAt"`
	UpdatedAt           time.Time      `json:"updatedAt"`
	DeletedAt           gorm.DeletedAt `gorm:"index" json:"deletedAt"`
}

// override the table name
func (TrustedUserInformation) TableName() string {
	return "users"
}

type M1Login struct {
	Email        string `json:"email"`
	LoginRequest string `json:"loginRequest"`
}

type M3Login struct {
	FinishLoginRequest string `json:"finishLoginRequest"`
	Nonce              string
}

type M2Login struct {
	Status        string `json:"status"`
	LoginResponse string `json:"loginResponse"`
	Nonce         string `json:"nonce"`
}
