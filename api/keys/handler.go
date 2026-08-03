package keys

import (
	"encoding/json"
	"net/http"
	"quartz/internal/model"

	"gorm.io/gorm"
)

type Handler struct {
	db *gorm.DB
}

func NewHandler(db *gorm.DB) *Handler {
	return &Handler{db: db}
}

func (h *Handler) GetUserKeys(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	email := r.URL.Query().Get("email")
	if email == "" {
		http.Error(w, "email is required", http.StatusBadRequest)
		return
	}

	// Find the user by email
	var user model.User
	if err := h.db.Where("email = ?", email).First(&user).Error; err != nil {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}

	// Fetch their public encryption keys
	var keyStore model.UserKeyStore
	if err := h.db.Where("user_id = ?", user.ID).First(&keyStore).Error; err != nil {
		http.Error(w, "keys not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"userId":                     user.ID.String(),
		"accountEncryptionPublicKey": keyStore.AccountEncryptionPublicKey,
	})
}
