package dto

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
)

// OPAQUE Message M1
type M1 struct {
	Email               string `json:"email"`
	RegistrationRequest string `json:"registrationRequest"`
}

// OPAQUE Message M2
type M2 struct {
	Status               string `json:"status"`
	RegistrationResponse string `json:"registrationResponse"`
	Nonce                string `json:"nonce"`
}

type UserDTO struct {
	Email string   `json:"email"`
	APAKE ApakeDTO `json:"aPAKE"`
	Keys  KeysDTO  `json:"keys"`
}

type ApakeDTO struct {
	RegistrationRecord string `json:"registrationRecord"`
	RegistrationNonce  string `json:"registrationNonce"`
}

type KeysDTO struct {
	MasterKdfSalt string `json:"masterKdfSalt"`

	AccountEncryptionPublicKey           string `json:"accountEncryptionPublicKey"`
	EncryptedAccountEncryptionPrivateKey string `json:"encAccountEncryptionPrivateKey"`
	AccountEncryptionKeyNonce            string `json:"accountEncryptionKeyNonce"`

	AccountSigningPublicKey           string `json:"accountSigningPublicKey"`
	EncryptedAccountSigningPrivateKey string `json:"encAccountSigningPrivateKey"`
	AccountSigningKeyNonce            string `json:"accountSigningKeyNonce"`
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

// OPAQUE Message M3
type M3 struct {
	User  UserDTO  `json:"user"`
	Drive DriveDTO `json:"drive"`
}
