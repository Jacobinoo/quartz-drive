package devices

import (
	"encoding/json"
	"log"
	"net/http"
	"quartz/internal/dto"
	"quartz/internal/middleware"
	"quartz/internal/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Handler struct {
	db *gorm.DB
}

func NewHandler(db *gorm.DB) *Handler {
	return &Handler{db: db}
}

func (h *Handler) RegisterDevice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 1. Get the current User ID from your Auth Middleware Context!
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	log.Printf("context2 %s", userID)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// 2. Decode the Payload
	var req dto.RegisterDeviceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json payload", http.StatusBadRequest)
		return
	}

	// 3. Find the most recent active session for this user
	// (If you have a way to extract the exact Session ID from the JWT, use that instead of First!)
	var session model.Session
	if err := h.db.Where("user_id = ?", userID).Order("created_at desc").First(&session).Error; err != nil {
		http.Error(w, "no active session found", http.StatusNotFound)
		return
	}

	// 4. Update the Session with the Ciphertext and Public Key!
	session.DevicePublicKey = req.DevicePublicKey
	session.WrappedAccountKeys = req.WrappedAccountKeys

	if err := h.db.Save(&session).Error; err != nil {
		log.Printf("Failed to register device keys: %v", err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "device_registered"})
}
