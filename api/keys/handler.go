package keys

import (
	"encoding/json"
	"net/http"
	"quartz/internal/model"
	apperrors "quartz/pkg/app-errors"
	"quartz/pkg/crypto"
	"strings"

	"gorm.io/gorm"
)

type Handler struct {
	db *gorm.DB
}

func NewHandler(db *gorm.DB) *Handler {
	return &Handler{db: db}
}

func (h *Handler) GetUserKeys(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodGet {
		return apperrors.NewMethodNotAllowed("method not allowed")
	}

	email := r.URL.Query().Get("email")
	if email == "" {
		return apperrors.NewBadRequest("email is required", nil)
	}

	email = strings.ToLower(email)

	hashedEmail, err := crypto.HashEmail([]byte(email))
	if err != nil {
		return apperrors.NewInternal(err)
	}

	// TODO: enumeration attack!
	// WARN: !
	// Find the user by email
	var user model.User
	if err := h.db.Where("hashed_email = ?", hashedEmail).First(&user).Error; err != nil {
		return apperrors.NewNotFound("user not found", err)
	}

	// Fetch their public encryption keys
	var keyStore model.UserKeyStore
	if err := h.db.Where("user_id = ?", user.ID).First(&keyStore).Error; err != nil {
		return apperrors.NewNotFound("keys not found", err)
	}

	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(map[string]string{
		"userId":                     user.ID.String(),
		"accountEncryptionPublicKey": keyStore.AccountEncryptionPublicKey,
	})
	if err != nil {
		return apperrors.NewInternal(err)
	}
	return nil
}
