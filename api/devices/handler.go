package devices

import (
	"encoding/json"
	"log"
	"net/http"
	"quartz/internal/dto"
	"quartz/internal/middleware"
	"quartz/internal/model"
	"quartz/pkg/dpop"
	"strings"

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

func (h *Handler) ListDevices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	dpopHeader := strings.TrimSpace(r.Header.Get("DPoP"))
	currentJkt, _ := dpop.ValidateDpopProof(dpopHeader, r)

	var refreshTokens []model.GormRefreshToken
	// Fetch all unrevoked refresh tokens and preload the associated session
	if err := h.db.Preload("Session").Where("user_id = ? AND is_revoked = ?", userID, false).Find(&refreshTokens).Error; err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}

	var devices []dto.DeviceResponse
	for _, rt := range refreshTokens {
		// Only include valid sessions
		if rt.Session.ID != uuid.Nil {
			devices = append(devices, dto.DeviceResponse{
				SessionID:    rt.Session.ID,
				DeviceName:   rt.Session.DeviceName,
				UserAgent:    rt.Session.UserAgent,
				LastActiveAt: rt.Session.LastActiveAt,
				CreatedAt:    rt.Session.CreatedAt,
				IsCurrent:    rt.DpopJKT == currentJkt,
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(devices)
}

func (h *Handler) RevokeDevice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req dto.RevokeDeviceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json payload", http.StatusBadRequest)
		return
	}

	// First, verify this session belongs to the user
	var token model.GormRefreshToken
	if err := h.db.Where("user_id = ? AND session_id = ?", userID, req.SessionID).First(&token).Error; err != nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	err := h.db.Transaction(func(tx *gorm.DB) error {
		// Delete the entire family of refresh tokens associated with this session
		if err := tx.Where("family_id = ?", token.FamilyID).Delete(&model.GormRefreshToken{}).Error; err != nil {
			return err
		}
		// Delete the session itself
		if err := tx.Where("id = ?", req.SessionID).Delete(&model.Session{}).Error; err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		log.Printf("failed to revoke device: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "device_revoked"})
}

