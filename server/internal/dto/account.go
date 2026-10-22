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
	PublicKey                         string `json:"publicKey" validate:"required,len=1622"`
	WrappedPrivateKey                 string `json:"wrappedPrivateKey" validate:"required,len=64"`
	PrivKeyNonce                      string `json:"privKeyNonce" validate:"required,len=32"`
	EncryptedPassphraseForOwner       string `json:"encryptedPassphraseForOwner" validate:"required,len=1590"`
	SignedEncryptedPassphraseForOwner string `json:"signedEncryptedPassphraseForOwner" validate:"required,len=86"`
}

type NodeDTO struct {
	PublicKey                 string `json:"publicKey" validate:"required,len=1622"`
	WrappedPrivateKey         string `json:"wrappedPrivateKey" validate:"required,len=64"`
	PrivKeyNonce              string `json:"privKeyNonce" validate:"required,len=32"`
	EncryptedPassphrase       string `json:"encryptedPassphrase" validate:"required,len=1590"`
	SignedEncryptedPassphrase string `json:"signedEncryptedPassphrase" validate:"required,len=86"`
}

type RedisPendingRegistration struct {
	UserID             uuid.UUID `json:"user_id"`
	EncryptedEmail     string    `json:"encrypted_email"`
	HashedEmail        string    `json:"hashed_email"`
	RegistrationRecord string    `json:"registration_record"`
}

type M1Reauthenticate struct {
	LoginRequest string `json:"loginRequest" validate:"required,len=128"`
}

type M2Reauthenticate struct {
	LoginResponse string `json:"loginResponse"`
	Email         string `json:"email"`
}
