package refresh

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"quartz/internal/model"
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

	csrfTokenHeader := r.Header.Get("X-CSRF-Token")
	if csrfTokenHeader == "" {
		http.Error(w, "unauthorized2", http.StatusUnauthorized)
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
	fmt.Println("Stored Token:", storedToken)             // Debugging line
	fmt.Println("Provided token hash:", refreshTokenHash) // Debugging line

	if storedToken.TokenHash != refreshTokenHash {
		fmt.Println("unauth4") // Debugging line
		http.Error(w, "unauthorized4", http.StatusUnauthorized)
		return
	}

	if storedToken.CsrfToken != csrfTokenHeader {
		fmt.Println("unauth5")                                    // Debugging line
		fmt.Println("Stored CSRF Token:", storedToken.CsrfToken)  // Debugging line
		fmt.Println("Provided CSRF Token:", csrfTokenHeader)      // Debugging line
		fmt.Println("Provided token header:", refreshTokenCookie) // Debugging line
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

	newAccessToken, expTime := token.IssueAccessToken(fingerprintHash)
	maxAge := time.Until(expTime)

	newCsrfToken := token.IssueCsrfToken()
	newRefreshToken := token.IssueRefreshToken()

	h.db.Create(&model.GormRefreshToken{
		TokenHash: newRefreshToken.TokenSha256Hash,
		UserID:    storedToken.UserID,
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
		CsrfToken: newCsrfToken.Token,
	})
	h.db.Delete(&storedToken)

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
		Status:    "ok",
		Token:     newAccessToken,
		CsrfToken: newCsrfToken.Token,
	}

	err = json.NewEncoder(w).Encode(refreshResponse)
	if err != nil {
		log.Fatalln(err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
}
