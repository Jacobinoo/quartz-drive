package refresh

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"quartz/internal/model"
	apperrors "quartz/pkg/app-errors"
	"quartz/pkg/crypto"
	"quartz/pkg/dpop"
	"quartz/pkg/token"
	"time"

	"gorm.io/gorm"
)

type Handler struct {
	db *gorm.DB
}

func NewHandler(db *gorm.DB) *Handler {
	return &Handler{db: db}
}

func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodPost {
		return apperrors.NewMethodNotAllowed("method not allowed")
	}

	refreshTokenCookie, err := r.Cookie("__Secure-Auth")
	if err != nil || refreshTokenCookie.Value == "" {
		return apperrors.NewUnauthorized("unauthorized1", err)
	}

	csrfTokenHeader := r.Header.Get("X-Csrf-Token")
	if csrfTokenHeader == "" {
		return apperrors.NewUnauthorized("unauthorized2", nil)
	}

	dpopProofHeader := r.Header.Get("DPoP")
	if dpopProofHeader == "" {
		return apperrors.NewUnauthorized("unauthorized-dpop-proof-missing", nil)
	}

	refreshTokenBytes, err := hex.DecodeString(refreshTokenCookie.Value)
	if err != nil {
		return apperrors.NewUnauthorized("unauthorized3", err)
	}

	refreshTokenHashBytes := sha256.Sum256(refreshTokenBytes)
	refreshTokenHash := hex.EncodeToString(refreshTokenHashBytes[:])

	var storedToken model.GormRefreshToken
	err = h.db.Where("token_hash = ?", refreshTokenHash).First(&storedToken).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperrors.NewUnauthorized("unauthorized", err)
		}
		return apperrors.NewInternal(err)
	}
	err = h.db.Preload("Session").Where("token_hash = ?", refreshTokenHash).First(&storedToken).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperrors.NewUnauthorized("unauthorized", err)
		}
		return apperrors.NewInternal(err)
	}

	slog.Debug("Stored Token:", storedToken.TokenHash)
	slog.Debug("Provided token hash:", refreshTokenHash)

	if storedToken.IsRevoked == true {
		slog.Info("token reuse detection triggered",
			"reused_revoked_token_id", storedToken.ID,
			"session_id", storedToken.SessionID,
			"family_id", storedToken.FamilyID,
			"user_id", storedToken.UserID,
		)
		err = h.db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Where("family_id = ?", storedToken.FamilyID).Delete(&model.GormRefreshToken{}).Error; err != nil {
				return err
			}

			if err := tx.Where("id = ?", storedToken.SessionID).Delete(&model.Session{}).Error; err != nil {
				return err
			}

			return nil
		})

		http.SetCookie(w, &http.Cookie{
			Name:     "__Secure-F",
			Value:    "",
			HttpOnly: true,
			Path:     "/",
			MaxAge:   int(-1),
			Secure:   true,
			SameSite: http.SameSiteLaxMode,
		})

		http.SetCookie(w, &http.Cookie{
			Name:     "__Secure-Auth",
			Value:    "",
			HttpOnly: true,
			Path:     "/",
			Secure:   true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   int(-1),
			Expires:  time.Unix(0, 0),
		})

		return apperrors.NewUnauthorized("unauthorized-reuse-detection", nil)
	}

	if subtle.ConstantTimeCompare([]byte(storedToken.TokenHash), []byte(refreshTokenHash)) == 0 {
		slog.Debug("refresh token hash mismatch",
			"stored_token_hash", storedToken.TokenHash,
			"provided_token_hash", refreshTokenHash,
		)
		return apperrors.NewUnauthorized("unauthorized4", nil)
	}

	providedJkt, err := dpop.ValidateDpopProof(dpopProofHeader, r)
	if err != nil {
		return apperrors.NewUnauthorized("dpop proof validation failed, could not derive thumbprint", err)
	}

	if providedJkt != storedToken.DpopJKT {
		return apperrors.NewUnauthorized("dpop key mismatch - token theft detected!", nil)
	}

	csrfBytes, err := hex.DecodeString(csrfTokenHeader)
	if err != nil {
		return apperrors.NewUnauthorized("invalid csrf hex", err)
	}

	hashBytes := sha256.Sum256([]byte(csrfBytes))
	providedCsrfHash := hex.EncodeToString(hashBytes[:])

	if subtle.ConstantTimeCompare([]byte(providedCsrfHash), []byte(storedToken.CsrfTokenHash)) != 1 {
		slog.Debug("unauth5")                                                         // Debugging line
		slog.Debug("Stored CSRF Token hash", "token_hash", storedToken.CsrfTokenHash) // Debugging line
		slog.Debug("Provided CSRF Token (hashed)", "token", providedCsrfHash)         // Debugging line
		slog.Debug("Provided token header", "header", refreshTokenCookie)             // Debugging line
		http.SetCookie(w, &http.Cookie{
			Name:     "__Secure-Auth",
			Value:    "",
			HttpOnly: true,
			Path:     "/",
			Secure:   true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   int(-1),
			Expires:  time.Unix(0, 0),
		})

		return apperrors.NewUnauthorized("unauthorized5", nil)
	}

	fingerprintBytes := make([]byte, 64)
	_, _ = rand.Read(fingerprintBytes)
	fingerprint := hex.EncodeToString(fingerprintBytes)

	fingerprintHashBytes := sha256.Sum256(fingerprintBytes)
	fingerprintHash := hex.EncodeToString(fingerprintHashBytes[:])

	var user model.User
	if err := h.db.Where("id = ?", storedToken.UserID).First(&user).Error; err != nil {
		return apperrors.NewUnauthorized("user not found", err)
	}

	decryptedEmail, err := crypto.DecryptEmail(user.EncryptedEmail)
	if err != nil {
		return apperrors.NewInternal(err)
	}

	// if EncryptionVersion is -1 , this means the account key material was not yet initialized
	// in this case the keys_initialized claim will be embedded inside the access token, to protect authenticated routes
	// that require the user to have his account key material initialized
	var withKeysInitialized = false
	if user.EncryptionVersion != -1 {
		withKeysInitialized = true
	}

	newAccessToken, expTime := token.IssueAccessToken(withKeysInitialized, fingerprintHash, storedToken.DpopJKT, storedToken.UserID.String(), string(decryptedEmail), storedToken.SessionID.String(), storedToken.FamilyID.String())
	maxAge := time.Until(expTime)

	newCsrfToken := token.IssueCsrfToken()
	newRefreshToken := token.IssueRefreshToken()

	err = h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&storedToken).Update("is_revoked", true).Error; err != nil {
			return err
		}

		var newToken model.GormRefreshToken
		// is this a transient session
		// We check < 25 hours (instead of 24) to account for minor millisecond differences between time.Now() and GORM's CreatedAt
		if storedToken.ExpiresAt.Sub(storedToken.CreatedAt) < time.Hour*25 {
			newToken = model.GormRefreshToken{
				TokenHash:     newRefreshToken.TokenSha256Hash,
				UserID:        storedToken.UserID,
				FamilyID:      storedToken.FamilyID,
				IsRevoked:     false,
				ExpiresAt:     time.Now().Add(24 * time.Hour),
				CsrfTokenHash: newCsrfToken.TokenSha256Hash,
				DpopJKT:       storedToken.DpopJKT,
				SessionID:     storedToken.SessionID,
			}
		} else {
			newToken = model.GormRefreshToken{
				TokenHash:     newRefreshToken.TokenSha256Hash,
				UserID:        storedToken.UserID,
				FamilyID:      storedToken.FamilyID,
				IsRevoked:     false,
				ExpiresAt:     time.Now().Add(7 * 24 * time.Hour),
				CsrfTokenHash: newCsrfToken.TokenSha256Hash,
				DpopJKT:       storedToken.DpopJKT,
				SessionID:     storedToken.SessionID,
			}
		}

		if err := tx.Create(&newToken).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return apperrors.NewInternal(fmt.Errorf("failed to rotate refresh token: %w", err))
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "__Secure-F",
		Value:    fingerprint,
		HttpOnly: true,
		Path:     "/",
		MaxAge:   int(maxAge.Seconds()),
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})

	// is this a transient session
	// We check < 25 hours (instead of 24) to account for minor millisecond differences between time.Now() and GORM's CreatedAt
	if storedToken.ExpiresAt.Sub(storedToken.CreatedAt) < time.Hour*25 {
		http.SetCookie(w, &http.Cookie{
			Name:     "__Secure-Auth",
			Value:    newRefreshToken.Token,
			HttpOnly: true,
			Path:     "/",
			Secure:   true,
			SameSite: http.SameSiteLaxMode,
			// transient session, don't add MaxAge or Expires
		})
	} else {
		http.SetCookie(w, &http.Cookie{
			Name:     "__Secure-Auth",
			Value:    newRefreshToken.Token,
			HttpOnly: true,
			Path:     "/",
			Secure:   true,
			SameSite: http.SameSiteLaxMode,
			Expires:  time.Now().Add(7 * 24 * time.Hour),
			MaxAge:   int(maxAge.Seconds()),
		})
	}

	var refreshResponse = RefreshResponse{
		Status:             "ok",
		Token:              newAccessToken,
		CsrfToken:          newCsrfToken.Token,
		WrappedAccountKeys: storedToken.Session.WrappedAccountKeys,
	}

	err = json.NewEncoder(w).Encode(refreshResponse)
	if err != nil {
		return apperrors.NewInternal(fmt.Errorf("failed to encode json: %w", err))
	}
	return nil
}
