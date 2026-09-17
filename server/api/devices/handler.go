package devices

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"quartz/internal/dto"
	"quartz/internal/model"
	apperrors "quartz/pkg/app-errors"
	"quartz/pkg/contextkeys"
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

func (h *Handler) RegisterDevice(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodPost {
		return apperrors.NewMethodNotAllowed("method not allowed")
	}

	// 1. Get the current User ID from your Auth Middleware Context!
	userID, ok := r.Context().Value(contextkeys.UserIDKey).(uuid.UUID)
	slog.Debug("context2")
	if !ok {
		return apperrors.NewUnauthorized("unauthorized", nil)
	}

	// 2. Decode the Payload
	var req dto.RegisterDeviceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return apperrors.NewBadRequest("invalid json payload", err)
	}

	// 3. Find the most recent active session for this user
	// (If you have a way to extract the exact Session ID from the JWT, use that instead of First!)
	var session model.Session
	if err := h.db.Where("user_id = ?", userID).Order("created_at desc").First(&session).Error; err != nil {
		return apperrors.NewNotFound("no active session found", err)
	}

	// 4. Update the Session with the Ciphertext and Public Key!
	session.DevicePublicKey = req.DevicePublicKey
	session.WrappedAccountKeys = req.WrappedAccountKeys

	if err := h.db.Save(&session).Error; err != nil {
		return apperrors.NewInternal(fmt.Errorf("failed to register device keys: %w", err))
	}

	w.WriteHeader(http.StatusOK)
	err := json.NewEncoder(w).Encode(map[string]string{"status": "device_registered"})
	if err != nil {
		return apperrors.NewInternal(err)
	}
	return nil
}

func (h *Handler) ListDevices(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodGet {
		return apperrors.NewMethodNotAllowed("method not allowed")
	}

	userID, ok := r.Context().Value(contextkeys.UserIDKey).(uuid.UUID)
	if !ok {
		return apperrors.NewUnauthorized("unauthorized", nil)
	}

	dpopHeader := strings.TrimSpace(r.Header.Get("DPoP"))
	currentJkt, _ := dpop.ValidateDpopProof(dpopHeader, r)

	var refreshTokens []model.GormRefreshToken
	// Fetch all unrevoked refresh tokens and preload the associated session
	if err := h.db.Preload("Session").Where("user_id = ? AND is_revoked = ?", userID, false).Find(&refreshTokens).Error; err != nil {
		return apperrors.NewInternal(fmt.Errorf("database error: %w", err))
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
	err := json.NewEncoder(w).Encode(devices)
	if err != nil {
		return apperrors.NewInternal(err)
	}
	return nil
}

func (h *Handler) RevokeDevice(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodDelete {
		return apperrors.NewMethodNotAllowed("method not allowed")
	}

	userID, ok := r.Context().Value(contextkeys.UserIDKey).(uuid.UUID)
	if !ok {
		return apperrors.NewUnauthorized("unauthorized", nil)
	}

	var req dto.RevokeDeviceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return apperrors.NewBadRequest("invalid json payload", err)
	}

	// First, verify this session belongs to the user
	var token model.GormRefreshToken
	if err := h.db.Where("user_id = ? AND session_id = ?", userID, req.SessionID).First(&token).Error; err != nil {
		return apperrors.NewNotFound("session not found", err)
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
		return apperrors.NewInternal(fmt.Errorf("failed to revoke device: %w", err))
	}

	w.WriteHeader(http.StatusOK)
	err = json.NewEncoder(w).Encode(map[string]string{"status": "device_revoked"})
	if err != nil {
		return apperrors.NewInternal(err)
	}
	return nil
}
