package dto

import "time"

type RecoveryCapability string

const (
	CapabilityAccount RecoveryCapability = "account"
	CapabilityData    RecoveryCapability = "data"
)

type RecoveryState string

const (
	StatePending  RecoveryState = "pending"
	StateVerified RecoveryState = "verified"
	StateComplete RecoveryState = "complete"
)

// RecoverySession is temporarily stored in Redis during the recovery flow
type RecoverySession struct {
	ID           string               `json:"id"`
	UserID       string               `json:"user_id,omitempty"`
	Method       string               `json:"method"`
	Capabilities []RecoveryCapability `json:"capabilities"`
	State        RecoveryState        `json:"state"`
	ExpiresAt    time.Time            `json:"expires_at"`

	// Method-specific verification state
	HashedToken *string `json:"hashed_token,omitempty"` // For Email Magic Links
	Challenge   *string `json:"challenge,omitempty"`    // For Phrase Challenge-Response
}

// Request/Response DTOs for the endpoints
type RecoveryStartRequest struct {
	Method string `json:"method" validate:"required,oneof=email phrase,max=255"`
	Email  string `json:"email,omitempty" validate:"required_if=Method email,omitempty,email,max=255"`

	// For Recovery Phrase
	RecoveryIDHex string `json:"recovery_id_hex,omitempty" validate:"required_if=Method phrase,omitempty,len=64"`
}

type RecoveryStartResponse struct {
	SessionID     string               `json:"session_id"`
	Capabilities  []RecoveryCapability `json:"capabilities"`
	Email         string               `json:"email,omitempty"`
	Challenge     *string              `json:"challenge,omitempty"`      // For Decrypt-to-Prove
	EncryptedKeys *KeysDTO             `json:"encrypted_keys,omitempty"` // Returns the keys to decrypt
}

type RecoveryVerifyRequest struct {
	// For Email
	Token string `json:"token,omitempty" validate:"required_without=ChallengeSignatureHex,excluded_with=ChallengeSignatureHex,omitempty,base64rawurl,len=64"`

	// For Recovery Phrase (Proof of possession)
	ChallengeSignatureHex string `json:"challenge_signature_hex,omitempty" validate:"required_without=Token,excluded_with=Token,omitempty,len=128"`
}

type RecoveryVerifyResponse struct {
	State        RecoveryState        `json:"state"`
	Capabilities []RecoveryCapability `json:"capabilities"`
}

type RecoveryOpaqueM1Request struct {
	RegistrationRequest string `json:"registrationRequest" validate:"required,len=43"`
}

type RecoveryCompleteRequest struct {
	NewOPAQUERecord *string  `json:"new_opaque_record,omitempty" validate:"required,len=256"`
	ReEncryptedKeys *KeysDTO `json:"re_encrypted_keys,omitempty"`
}
