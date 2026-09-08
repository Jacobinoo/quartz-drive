package dto

import (
	"database/sql/driver"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
)

type InitializeAccountKeysRequest struct {
	User struct {
		Keys KeysDTO `json:"keys"`
	} `json:"user"`
	Drive DriveDTO `json:"drive"`
}

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

type DriveDTO struct {
	DefaultShare ShareDTO `json:"defaultShare"`
	RootNode     NodeDTO  `json:"rootNode"`
}

type ShareDTO struct {
	PublicKey                         string `json:"publicKey"`
	WrappedPrivateKey                 string `json:"wrappedPrivateKey"`
	PrivKeyNonce                      string `json:"privKeyNonce"`
	EncryptedPassphraseForOwner       string `json:"encryptedPassphraseForOwner"`
	SignedEncryptedPassphraseForOwner string `json:"signedEncryptedPassphraseForOwner"`
}

type NodeDTO struct {
	PublicKey                 string `json:"publicKey"`
	WrappedPrivateKey         string `json:"wrappedPrivateKey"`
	PrivKeyNonce              string `json:"privKeyNonce"`
	EncryptedPassphrase       string `json:"encryptedPassphrase"`
	SignedEncryptedPassphrase string `json:"signedEncryptedPassphrase"`
}

type RedisPendingRegistration struct {
	UserID             uuid.UUID `json:"user_id"`
	EncryptedEmail     string    `json:"encrypted_email"`
	HashedEmail        string    `json:"hashed_email"`
	RegistrationRecord string    `json:"registration_record"`
}

type M1Reauthenticate struct {
	LoginRequest string `json:"loginRequest"`
}

type M2Reauthenticate struct {
	LoginResponse string `json:"loginResponse"`
	Email         string `json:"email"`
}

type RedisPendingEmailRecovery struct {
	UserID    uuid.UUID `json:"user_id"`
	TokenHash string    `json:"token_hash"`
}
