package dto

import "github.com/google/uuid"

type UserRegistrationRecord struct {
	CredentialID       uuid.UUID `gorm:"column:id;type:uuid"`
	RegistrationRecord string    `gorm:"column:registration_record"`
	EncryptedEmail     string    `gorm:"column:encrypted_email"`
}

// override the table name
func (UserRegistrationRecord) TableName() string {
	return "users"
}
