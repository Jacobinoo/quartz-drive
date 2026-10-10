package dto

// OPAQUE Message M1 - Request from user
type M1 struct {
	Email               string `json:"email" validate:"required,email,max=255"`
	RegistrationRequest string `json:"registrationRequest" validate:"required,len=43"`
}

// OPAQUE Message M2 - Response from server
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
	Email string   `json:"email" validate:"required,email,max=255"`
	APAKE ApakeDTO `json:"aPAKE"`
}

type ApakeDTO struct {
	RegistrationRecord string `json:"registrationRecord" validate:"required,len=256"`
	RegistrationNonce  string `json:"registrationNonce" validate:"required,len=43"`
}
