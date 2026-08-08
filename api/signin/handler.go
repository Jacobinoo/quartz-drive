package signin

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"quartz/internal/bindings"
	"quartz/internal/dto"
	"quartz/internal/model"
	"quartz/pkg/dpop"
	"quartz/pkg/token"

	// pb "quartz/proto"
	"time"

	"github.com/google/uuid"
	"github.com/mssola/useragent"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type Handler struct {
	db          *gorm.DB
	redis       *redis.Client
	opaqueSetup []byte
}

func NewHandler(db *gorm.DB, redisClient *redis.Client, opaqueSetup []byte) *Handler {
	return &Handler{db: db, redis: redisClient, opaqueSetup: opaqueSetup}
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var m1 dto.M1Login
	if err := json.NewDecoder(r.Body).Decode(&m1); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var userRegistrationRecord dto.UserRegistrationRecord
	if err := h.db.Where("email = ?", m1.Email).First(&userRegistrationRecord).Error; err != nil {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}

	regRecordBytes, err := base64.RawURLEncoding.DecodeString(userRegistrationRecord.RegistrationRecord)
	if err != nil {
		fmt.Print(err)
		http.Error(w, "invalid registration record in database", http.StatusInternalServerError)
		return
	}

	loginReqBytes, err := base64.RawURLEncoding.DecodeString(m1.LoginRequest)
	if err != nil {
		http.Error(w, "invalid base64 in login request", http.StatusBadRequest)
		return
	}

	log.Printf("DEBUG: opaqueSetup len=%d", len(h.opaqueSetup))
	log.Printf("DEBUG: regRecordBytes len=%d", len(regRecordBytes))
	log.Printf("DEBUG: loginReqBytes len=%d", len(loginReqBytes))
	log.Printf("DEBUG: credentialID len=%d", len(userRegistrationRecord.CredentialID.String()))

	startLogRes, serverLoginState, err := bindings.StartLogin(
		h.opaqueSetup,
		regRecordBytes,
		loginReqBytes,
		[]byte(userRegistrationRecord.CredentialID.String()),
	)

	var m2 dto.M2Login

	if err == nil {
		// Generate nonce
		nonceBytes := make([]byte, 32)
		rand.Read(nonceBytes)
		nonce := hex.EncodeToString(nonceBytes)

		// Store in Redis
		nonceData := map[string]string{
			"serverLoginState": base64.RawURLEncoding.EncodeToString(serverLoginState),
			"email":            m1.Email,
		}
		nonceJSON, _ := json.Marshal(nonceData)
		h.redis.Set(context.Background(), "login:nonce:"+nonce, nonceJSON, 30*time.Second)

		m2 = dto.M2Login{
			Status:        "ok",
			LoginResponse: base64.RawURLEncoding.EncodeToString(startLogRes),
			Nonce:         nonce,
		}
	} else {
		log.Printf("bindings StartLogin call failed: %v", err)
		m2 = dto.M2Login{
			Status:        "not_ok",
			LoginResponse: "",
			Nonce:         "",
		}
	}

	err = json.NewEncoder(w).Encode(m2)
	if err != nil {
		log.Fatalln(err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
}

func (h *Handler) LoginM3(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	dpopHeader := r.Header.Get("DPoP")
	thumbprint, err := dpop.ValidateDpopProof(dpopHeader, r)
	if err != nil {
		log.Printf("invalid dpop proof, err: %v", err)
		http.Error(w, "invalid_dpop_proof", http.StatusBadRequest)
		return
	}

	var m3 dto.M3Login
	if err := json.NewDecoder(r.Body).Decode(&m3); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Fetch from Redis
	nonceJSON, err := h.redis.Get(context.Background(), "login:nonce:"+m3.Nonce).Result()
	if err != nil {
		http.Error(w, "invalid or expired nonce", http.StatusBadRequest)
		return
	}

	var nonceData map[string]string
	json.Unmarshal([]byte(nonceJSON), &nonceData)

	serverLoginState, _ := base64.RawURLEncoding.DecodeString(nonceData["serverLoginState"])
	email := nonceData["email"]

	finishLoginReqBytes, err := base64.RawURLEncoding.DecodeString(m3.FinishLoginRequest)
	if err != nil {
		http.Error(w, "invalid base64 in finish login request", http.StatusBadRequest)
		return
	}

	sessionKey, err := bindings.FinishLogin(
		serverLoginState,
		finishLoginReqBytes,
	)

	if err != nil {
		log.Printf("bindings FinishLogin call failed: %v", err)
		http.Error(w, "login failed", http.StatusUnauthorized)
		return
	}

	fmt.Println("Established a new trusted session key: ", hex.EncodeToString(sessionKey))

	var trustedUserInfo dto.TrustedUserInformation

	err = h.db.Model(&model.User{}).
		Select("users.*, user_key_stores.*").
		Joins("INNER JOIN user_key_stores ON user_key_stores.user_id = users.id").
		Where("users.email = ?", email).
		First(&trustedUserInfo).Error

	if err != nil {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}

	uaHeader := r.Header.Get("User-Agent")
	ua := useragent.New(uaHeader)

	fmt.Printf("Mobile: %v\n", ua.Mobile())   // => true
	fmt.Printf("Bot: %v\n", ua.Bot())         // => false
	fmt.Printf("Mozilla: %v\n", ua.Mozilla()) // => "5.0"
	fmt.Printf("Model: %v\n", ua.Model())     // => "Nexus One"

	fmt.Printf("Platform: %v\n", ua.Platform()) // => "Linux"
	fmt.Printf("OS: %v\n", ua.OS())             // => "Android 2.3.7"

	name, version := ua.Engine()
	fmt.Printf("Engine: %v\n", name)     // => "AppleWebKit"
	fmt.Printf("Version: %v\n", version) // => "533.1"

	name, version = ua.Browser()
	fmt.Printf("Browser: %v\n", name)    // => "Android"
	fmt.Printf("Version: %v\n", version) // => "4.0"

	displayedDeviceName := ua.OS() + " - " + ua.Model() + " - " + name

	newSessionEntry := model.Session{
		UserID:       trustedUserInfo.ID,
		UserAgent:    uaHeader,
		DeviceName:   displayedDeviceName,
		LastActiveAt: time.Now(),
	}

	familyID := uuid.New()
	refreshToken := token.IssueRefreshToken()
	newCsrfToken := token.IssueCsrfToken()

	refreshTokenEntry := model.GormRefreshToken{
		UserID:        trustedUserInfo.ID,
		TokenHash:     refreshToken.TokenSha256Hash,
		FamilyID:      familyID,
		IsRevoked:     false,
		ExpiresAt:     time.Now().Add(7 * 24 * time.Hour), // default 7 days
		CsrfTokenHash: newCsrfToken.TokenSha256Hash,
		Session:       newSessionEntry,
		DpopJKT:       thumbprint,
	}

	fingerprintBytes := make([]byte, 64)
	_, _ = rand.Read(fingerprintBytes)
	fingerprint := hex.EncodeToString(fingerprintBytes)

	fingerprintHashBytes := sha256.Sum256(fingerprintBytes)
	fingerprintHash := hex.EncodeToString(fingerprintHashBytes[:])

	accessToken, expTime := token.IssueAccessToken(fingerprintHash, thumbprint, trustedUserInfo.ID.String(), trustedUserInfo.Email)
	maxAge := time.Until(expTime)

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
		Value:    refreshToken.Token,
		HttpOnly: true,
		Path:     "/",
		Secure:   true,                  // change to true in production
		SameSite: http.SameSiteNoneMode, // change to http.SameSiteStrictMode in production
		Expires:  time.Now().Add(7 * 24 * time.Hour),
	})

	if err := h.db.Create(&refreshTokenEntry).Error; err != nil {
		log.Printf("failed to store login data: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	var loginTrustAttestation = dto.LoginTrustAttestationConfirmedBody{
		LoginTrustAttestationHeader: dto.LoginTrustAttestationHeader{
			Status:      "ok",
			Attestation: true,
		},
		TrustedUserInformation: trustedUserInfo,
		Token:                  accessToken,
		CsrfToken:              newCsrfToken.Token,
	}

	err = json.NewEncoder(w).Encode(loginTrustAttestation)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		// loginSessions.Delete(m3.Nonce)
		return
	}
}
