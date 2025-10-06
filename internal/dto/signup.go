package dto

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

// OPAQUE Message M3
type M3 struct {
	Email string `json:"email" gorm:"uniqueIndex"`

	Salt                string `json:"salt"`
	PublicKey           string `json:"publicKey"`
	EncryptedPrivateKey string `json:"encryptedPrivateKey"`
	Nonce               string `json:"nonce"`

	RegistrationRecord string `json:"registrationRecord"`
	RegistrationNonce  string `gorm:"-" json:"registrationNonce"`
}
