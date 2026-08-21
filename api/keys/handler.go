package keys

import (
	"encoding/json"
	"net/http"
	"quartz/internal/model"
	apperrors "quartz/pkg/app-errors"

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

	// Find the user by email
	var user model.User
	if err := h.db.Where("email = ?", email).First(&user).Error; err != nil {
		return apperrors.NewNotFound("user not found", err)
	}

	// Fetch their public encryption keys
	var keyStore model.UserKeyStore
	if err := h.db.Where("user_id = ?", user.ID).First(&keyStore).Error; err != nil {
		return apperrors.NewNotFound("keys not found", err)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"userId":                     user.ID.String(),
		"accountEncryptionPublicKey": keyStore.AccountEncryptionPublicKey,
	})
	return nil
}
