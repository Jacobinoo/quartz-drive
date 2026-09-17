package dto

import (
	"github.com/google/uuid"
)

type InitializeAccountKeysRequest struct {
	User struct {
		Keys KeysDTO `json:"keys"`
	} `json:"user"`
	Drive DriveDTO `json:"drive"`
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
