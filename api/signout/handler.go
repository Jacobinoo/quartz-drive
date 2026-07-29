package signout

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"quartz/internal/model"

	"gorm.io/gorm"
)

type Handler struct {
	db *gorm.DB
}

func NewHandler(db *gorm.DB) *Handler {
	return &Handler{db: db}
}

func (h *Handler) Signout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 1. Always send headers to destroy browser cookies immediately!
	http.SetCookie(w, &http.Cookie{
		Name:     "__Secure-F",
		Value:    "",
		HttpOnly: true,
		Path:     "/",
		MaxAge:   -1,
		Secure:   true,
		SameSite: http.SameSiteNoneMode,
		Expires:  time.Unix(0, 0),
	})

	http.SetCookie(w, &http.Cookie{
		Name:     "__Secure-Auth",
		Value:    "",
		HttpOnly: true,
		Path:     "/",
		MaxAge:   -1,
		Secure:   true,
		SameSite: http.SameSiteNoneMode,
		Expires:  time.Unix(0, 0),
	})

	csrfTokenHeader := r.Header.Get("X-Csrf-Token")
	if csrfTokenHeader == "" {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		return
	}

	// 2. Read cookie to revoke the session in the database
	cookie, err := r.Cookie("__Secure-Auth")
	if err != nil || cookie.Value == "" {
		// Even if cookie is missing, we still return 200 OK because cookies were wiped above
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		return
	}

	tokenBytes, err := hex.DecodeString(cookie.Value)
	if err == nil {
		hash := sha256.Sum256(tokenBytes)
		tokenHash := hex.EncodeToString(hash[:])

		var storedToken model.GormRefreshToken
		if err := h.db.Where("token_hash = ?", tokenHash).First(&storedToken).Error; err == nil {
			// 3. Destroy the entire token family AND the session record!
			h.db.Transaction(func(tx *gorm.DB) error {
				tx.Where("family_id = ?", storedToken.FamilyID).Delete(&model.GormRefreshToken{})
				tx.Where("id = ?", storedToken.SessionID).Delete(&model.Session{})
				return nil
			})
		}
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
