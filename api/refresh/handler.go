package refresh

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"quartz/internal/model"
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

func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	refreshTokenCookie, err := r.Cookie("__Secure-Auth")
	if err != nil || refreshTokenCookie.Value == "" {
		http.Error(w, "unauthorized1", http.StatusUnauthorized)
		return
	}

	csrfTokenHeader := r.Header.Get("X-Csrf-Token")
	if csrfTokenHeader == "" {
		http.Error(w, "unauthorized2", http.StatusUnauthorized)
		return
	}

	dpopProofHeader := r.Header.Get("DPoP")
	if dpopProofHeader == "" {
		http.Error(w, "unauthorized-dpop-proof-missing", http.StatusUnauthorized)
		return
	}

	refreshTokenBytes, err := hex.DecodeString(refreshTokenCookie.Value)
	if err != nil {
		http.Error(w, "unauthorized3", http.StatusUnauthorized)
		return
	}

	refreshTokenHashBytes := sha256.Sum256(refreshTokenBytes)
	refreshTokenHash := hex.EncodeToString(refreshTokenHashBytes[:])

	var storedToken model.GormRefreshToken
	h.db.Where("token_hash = ?", refreshTokenHash).First(&storedToken)
	h.db.Preload("Session").Where("token_hash = ?", refreshTokenHash).First(&storedToken)

	fmt.Println("Stored Token:", storedToken)             // Debugging line
	fmt.Println("Provided token hash:", refreshTokenHash) // Debugging line

	if storedToken.IsRevoked == true {
		fmt.Println("token reuse detections triggered!")
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
			Secure:   true,                  // change to true in production
			SameSite: http.SameSiteNoneMode, // change to http.SameSiteStrictMode in production
		})

		http.SetCookie(w, &http.Cookie{
			Name:     "__Secure-Auth",
			Value:    "",
			HttpOnly: true,
			Path:     "/",
			Secure:   true,                  // change to true in production
			SameSite: http.SameSiteNoneMode, // change to http.SameSiteStrictMode in production
			Expires:  time.Unix(0, 0),
		})

		http.Error(w, "unauthorized-reuse-detection", http.StatusUnauthorized)
		return
	}

	if storedToken.TokenHash != refreshTokenHash {
		fmt.Println("unauth4") // Debugging line
		http.Error(w, "unauthorized4", http.StatusUnauthorized)
		return
	}

	providedJkt, err := dpop.ValidateDpopProof(dpopProofHeader, r)
	if err != nil {
		http.Error(w, "dpop proof validation failed, could not derive thumbprint", http.StatusUnauthorized)
		return
	}

	if providedJkt != storedToken.DpopJKT {
		http.Error(w, "dpop key mismatch - token theft detected!", http.StatusUnauthorized)
		return
	}

	csrfBytes, err := hex.DecodeString(csrfTokenHeader)
	if err != nil {
		http.Error(w, "invalid csrf hex", http.StatusUnauthorized)
		return
	}

	hashBytes := sha256.Sum256([]byte(csrfBytes))
	providedCsrfHash := hex.EncodeToString(hashBytes[:])

	if subtle.ConstantTimeCompare([]byte(providedCsrfHash), []byte(storedToken.CsrfTokenHash)) != 1 {
		fmt.Println("unauth5")                                            // Debugging line
		fmt.Println("Stored CSRF Token hash:", storedToken.CsrfTokenHash) // Debugging line
		fmt.Println("Provided CSRF Token (hashed):", providedCsrfHash)    // Debugging line
		fmt.Println("Provided token header:", refreshTokenCookie)         // Debugging line
		http.SetCookie(w, &http.Cookie{
			Name:     "__Secure-Auth",
			Value:    "",
			HttpOnly: true,
			Path:     "/",
			Secure:   true,                  // change to true in production
			SameSite: http.SameSiteNoneMode, // change to http.SameSiteStrictMode in production
			Expires:  time.Unix(0, 0),
		})

		http.Error(w, "unauthorized5", http.StatusUnauthorized)
		return
	}

	fingerprintBytes := make([]byte, 64)
	rand.Read(fingerprintBytes)
	fingerprint := hex.EncodeToString(fingerprintBytes)

	fingerprintHashBytes := sha256.Sum256(fingerprintBytes)
	fingerprintHash := hex.EncodeToString(fingerprintHashBytes[:])

	var user model.User
	if err := h.db.Where("id = ?", storedToken.UserID).First(&user).Error; err != nil {
		http.Error(w, "user not found", http.StatusUnauthorized)
		return
	}

	newAccessToken, expTime := token.IssueAccessToken(fingerprintHash, storedToken.DpopJKT, storedToken.UserID.String(), user.Email)
	maxAge := time.Until(expTime)

	newCsrfToken := token.IssueCsrfToken()
	newRefreshToken := token.IssueRefreshToken()

	err = h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&storedToken).Update("is_revoked", true).Error; err != nil {
			return err
		}

		newToken := model.GormRefreshToken{
			TokenHash:     newRefreshToken.TokenSha256Hash,
			UserID:        storedToken.UserID,
			FamilyID:      storedToken.FamilyID,
			IsRevoked:     false,
			ExpiresAt:     time.Now().Add(7 * 24 * time.Hour),
			CsrfTokenHash: newCsrfToken.TokenSha256Hash,
			DpopJKT:       storedToken.DpopJKT,
			SessionID:     storedToken.SessionID,
		}

		if err := tx.Create(&newToken).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		log.Printf("failed to rotate refresh token: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "__Secure-F",
		Value:    fingerprint,
		HttpOnly: true,
		Path:     "/",
		MaxAge:   int(maxAge.Seconds()),
		Secure:   true,                  // change to true in production
		SameSite: http.SameSiteNoneMode, // change to http.SameSiteStrictMode in production
	})

	http.SetCookie(w, &http.Cookie{
		Name:     "__Secure-Auth",
		Value:    newRefreshToken.Token,
		HttpOnly: true,
		Path:     "/",
		Secure:   true,                  // change to true in production
		SameSite: http.SameSiteNoneMode, // change to http.SameSiteStrictMode in production
		Expires:  time.Now().Add(7 * 24 * time.Hour),
	})

	var refreshResponse = RefreshResponse{
		Status:            "ok",
		Token:             newAccessToken,
		CsrfToken:         newCsrfToken.Token,
		SessionPrivateKey: storedToken.Session.SessionPrivateKey,
	}

	err = json.NewEncoder(w).Encode(refreshResponse)
	if err != nil {
		log.Fatalln(err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
}
