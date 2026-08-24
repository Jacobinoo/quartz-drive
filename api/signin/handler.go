package signin

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"quartz/internal/bindings"
	"quartz/internal/dto"
	"quartz/internal/model"
	"quartz/pkg/app-errors"
	"quartz/pkg/captcha"
	"quartz/pkg/crypto"
	"quartz/pkg/dpop"
	"quartz/pkg/token"
	"strings"

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

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) error {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		return apperrors.NewMethodNotAllowed("method not allowed")
	}

	var m1 dto.M1Login
	if err := json.NewDecoder(r.Body).Decode(&m1); err != nil {
		return apperrors.NewBadRequest("invalid request", err)
	}

	loginReqBytes, err := base64.RawURLEncoding.DecodeString(m1.LoginRequest)
	if err != nil {
		return apperrors.NewBadRequest("invalid base64 in login request", err)
	}

	success, errArr, err := captcha.VerifyTurnstileTokenInRequest(r)
	if err != nil {
		slog.WarnContext(r.Context(), "turnstile verification failed", "errors", strings.Join(errArr, ","), "error", err)
		return apperrors.NewBadRequest("invalid token", err)
	}
	if !success {
		slog.WarnContext(r.Context(), "turnstile verification failed", "errors", strings.Join(errArr, ","))
		return apperrors.NewBadRequest("invalid token", nil)
	}

	hashedEmail, err := crypto.HashEmail([]byte(m1.Email))
	if err != nil {
		return apperrors.NewInternal(err)
	}

	var userRegistrationRecord dto.UserRegistrationRecord
	if err := h.db.Where("hashed_email = ?", hashedEmail).First(&userRegistrationRecord).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperrors.NewNotFound("user not found", err)
		}
		return apperrors.NewInternal(err)
	}

	regRecordBytes, err := base64.RawURLEncoding.DecodeString(userRegistrationRecord.RegistrationRecord)
	if err != nil {
		return apperrors.NewInternal(err)
	}

	slog.Debug("DEBUG: opaqueSetup", "len", len(h.opaqueSetup))
	slog.Debug("DEBUG: regRecordBytes", "len", len(regRecordBytes))
	slog.Debug("DEBUG: loginReqBytes", "len", len(loginReqBytes))
	slog.Debug("DEBUG: credentialID", "len", len(userRegistrationRecord.CredentialID.String()))

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
		_, _ = rand.Read(nonceBytes)
		nonce := hex.EncodeToString(nonceBytes)

		// Store in Redis
		nonceData := map[string]string{
			"serverLoginState": base64.RawURLEncoding.EncodeToString(serverLoginState),
			"email":            hashedEmail,
		}
		nonceJSON, err := json.Marshal(nonceData)
		if err != nil {
			return apperrors.NewInternal(err)
		}

		encryptedJSON, err := crypto.EncryptRedisPayload(nonceJSON)
		if err != nil {
			return apperrors.NewInternal(err)
		}

		set := h.redis.Set(context.Background(), "login:nonce:"+nonce, encryptedJSON, 30*time.Second)
		if set.Err() != nil {
			return apperrors.NewInternal(err)
		}

		m2 = dto.M2Login{
			Status:        "ok",
			LoginResponse: base64.RawURLEncoding.EncodeToString(startLogRes),
			Nonce:         nonce,
		}
	} else {
		slog.DebugContext(r.Context(), "bindings StartLogin call failed: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		m2 = dto.M2Login{
			Status:        "not_ok",
			LoginResponse: "",
			Nonce:         "",
		}
	}

	err = json.NewEncoder(w).Encode(m2)
	if err != nil {
		return apperrors.NewInternal(err)
	}
	return nil
}

func (h *Handler) LoginM3(w http.ResponseWriter, r *http.Request) error {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		return apperrors.NewMethodNotAllowed("method not allowed")
	}

	dpopHeader := r.Header.Get("DPoP")
	thumbprint, err := dpop.ValidateDpopProof(dpopHeader, r)
	if err != nil {
		slog.InfoContext(r.Context(), "invalid dpop proof", "error", err)
		return apperrors.NewBadRequest("invalid dpop proof", err)
	}

	var m3 dto.M3Login
	if err := json.NewDecoder(r.Body).Decode(&m3); err != nil {
		return apperrors.NewBadRequest("invalid request", err)
	}

	// Fetch from Redis
	encryptedNonceJSON, err := h.redis.Get(context.Background(), "login:nonce:"+m3.Nonce).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return apperrors.NewBadRequest("invalid or expired nonce", err)
		}
		return apperrors.NewInternal(err)
	}

	nonceJSONBytes, err := crypto.DecryptRedisPayload(encryptedNonceJSON)
	if err != nil {
		return apperrors.NewInternal(err)
	}

	var nonceData map[string]string
	err = json.Unmarshal(nonceJSONBytes, &nonceData)
	if err != nil {
		return apperrors.NewInternal(err)
	}

	serverLoginState, _ := base64.RawURLEncoding.DecodeString(nonceData["serverLoginState"])
	hashedEmail := nonceData["email"]

	finishLoginReqBytes, err := base64.RawURLEncoding.DecodeString(m3.FinishLoginRequest)
	if err != nil {
		http.Error(w, "invalid base64 in finish login request", http.StatusBadRequest)
		return apperrors.NewBadRequest("invalid base64 in finish login request", err)
	}

	_, err = bindings.FinishLogin(
		serverLoginState,
		finishLoginReqBytes,
	)

	if err != nil {
		slog.DebugContext(r.Context(), "bindings FinishLogin call failed: %v", err)
		return apperrors.NewUnauthorized("login failed", err)
	}

	slog.Debug("Established a new trusted session key.")

	var trustedUserInfo dto.TrustedUserInformation

	err = h.db.Model(&model.User{}).
		Select("users.*, users.encrypted_email AS email, user_key_stores.*").
		Joins("INNER JOIN user_key_stores ON user_key_stores.user_id = users.id").
		Where("users.hashed_email = ?", hashedEmail).
		First(&trustedUserInfo).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperrors.NewNotFound("user not found", err)
		}
		return apperrors.NewInternal(err)
	}

	// The database does not store an email in plaintext, instead we use two columns
	// that store an HMAC hash of the email, and an AES-GCM ciphertext.
	// This is brilliant, because it protects the PII in the DB, while still allowing for email sending & lookups
	// The dto has an "Email" field, it will be null, because there is no column of that name in the database
	// Instead we need to decrypt the email and return the decrypted email to the user
	decryptedEmail, err := crypto.DecryptEmail(trustedUserInfo.Email)
	if err != nil {
		return apperrors.NewInternal(err)
	}
	trustedUserInfo.Email = string(decryptedEmail)

	uaHeader := r.Header.Get("User-Agent")
	if uaHeader == "" {
		return apperrors.NewBadRequest("invalid user agent", err)
	}
	ua := useragent.New(uaHeader)

	name, _ := ua.Browser()

	displayedDeviceName := ua.OS() + " - " + ua.Model() + " - " + name

	generatedNewSessionID := uuid.New()

	newSessionEntry := model.Session{
		ID:           generatedNewSessionID,
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

	accessToken, expTime := token.IssueAccessToken(fingerprintHash, thumbprint, trustedUserInfo.ID.String(), trustedUserInfo.Email, generatedNewSessionID.String(), familyID.String())
	maxAge := time.Until(expTime)

	http.SetCookie(w, &http.Cookie{
		Name:     "__Secure-F",
		Value:    fingerprint,
		HttpOnly: true,
		Path:     "/",
		MaxAge:   int(maxAge.Seconds()),
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})

	http.SetCookie(w, &http.Cookie{
		Name:     "__Secure-Auth",
		Value:    refreshToken.Token,
		HttpOnly: true,
		Path:     "/",
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(7 * 24 * time.Hour),
	})

	if err := h.db.Create(&refreshTokenEntry).Error; err != nil {
		return apperrors.NewInternal(err)
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
		// loginSessions.Delete(m3.Nonce)
		return apperrors.NewInternal(err)
	}
	return nil
}
