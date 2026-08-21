package middleware

import (
	"net/http"
	"quartz/internal/model"
	"quartz/pkg/dpop"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"quartz/pkg/contextkeys"
	"quartz/pkg/httputils"
)

// LastActivityTracker updates the last_active_at timestamp for the current session in the background
func LastActivityTracker(db *gorm.DB, next httputils.APIHandler) httputils.APIHandler {
	return func(w http.ResponseWriter, r *http.Request) error {
		// Extract UserID from context (set by AccessTokenMiddleware)
		userID, ok := r.Context().Value(contextkeys.UserIDKey).(uuid.UUID)
		if ok {
			// We also need the JKT to pinpoint exactly WHICH session is active
			dpopHeader := strings.TrimSpace(r.Header.Get("DPoP"))
			jkt, err := dpop.ValidateDpopProof(dpopHeader, r)

			if err == nil && jkt != "" {
				// Run in the background! Never blocks the main request thread and never fails the request.
				go func(uid uuid.UUID, dpopJkt string) {
					var token model.GormRefreshToken
					// Find the session ID for this specific device (DPoP JKT)
					if err := db.Where("user_id = ? AND dpop_jkt = ? AND is_revoked = ?", uid, dpopJkt, false).First(&token).Error; err == nil {
						// Update the session's LastActiveAt timestamp
						db.Model(&model.Session{}).Where("id = ?", token.SessionID).Update("last_active_at", time.Now())
					}
				}(userID, jkt)
			}
		}

		return next(w, r)
	}
}
