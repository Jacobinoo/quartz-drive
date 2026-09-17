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
	User UserDTO `json:"user"`
}

type UserDTO struct {
	Email string   `json:"email"`
	APAKE ApakeDTO `json:"aPAKE"`
}

type ApakeDTO struct {
	RegistrationRecord string `json:"registrationRecord"`
	RegistrationNonce  string `json:"registrationNonce"`
}
