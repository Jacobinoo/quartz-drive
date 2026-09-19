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
	"quartz/pkg/contextkeys"
	"quartz/pkg/crypto"
	"quartz/pkg/dpop"
	"quartz/pkg/token"
	"strconv"
	"strings"

	// pb "quartz/proto"
	"time"

	"github.com/google/uuid"
	"github.com/mssola/useragent"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type Handler struct {
	db                           *gorm.DB
	redis                        *redis.Client
	opaqueSetup                  []byte
	fakeOpaqueRegistrationRecord []byte
}

func NewHandler(db *gorm.DB, redisClient *redis.Client, opaqueSetup []byte, fakeOpaqueRegistrationRecord []byte) *Handler {
	return &Handler{db: db, redis: redisClient, opaqueSetup: opaqueSetup, fakeOpaqueRegistrationRecord: fakeOpaqueRegistrationRecord}
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

	m1.Email = strings.ToLower(m1.Email)

	hashedEmailHex, err := crypto.HashEmail([]byte(m1.Email))
	if err != nil {
		return apperrors.NewInternal(err)
	}

	var userRegistrationRecord dto.UserRegistrationRecord
	err = h.db.Where("hashed_email = ?", hashedEmailHex).First(&userRegistrationRecord).Error

	var registrationRecordBytes []byte
	var credentialIDBytes []byte
	isFake := false

	if errors.Is(err, gorm.ErrRecordNotFound) {
		// User doesn't exist — use fake record
		registrationRecordBytes = h.fakeOpaqueRegistrationRecord
		credentialIDBytes = []byte("fake-user-identifier")
		isFake = true
	} else if err != nil {
		return apperrors.NewInternal(err)
	} else {
		// User exists — use real record
		registrationRecordBytes, err = base64.RawURLEncoding.DecodeString(userRegistrationRecord.RegistrationRecord)
		if err != nil {
			return apperrors.NewInternal(err)
		}
		credentialIDBytes = []byte(userRegistrationRecord.CredentialID.String())
	}

	slog.Debug("DEBUG: opaqueSetup", "len", len(h.opaqueSetup))
	slog.Debug("DEBUG: regRecordBytes", "len", len(registrationRecordBytes))
	slog.Debug("DEBUG: loginReqBytes", "len", len(loginReqBytes))
	slog.Debug("DEBUG: credentialID", "len", len(credentialIDBytes))

	startLogRes, serverLoginState, err := bindings.StartLogin(
		h.opaqueSetup,
		registrationRecordBytes,
		loginReqBytes,
		credentialIDBytes,
	)

	var m2 dto.M2Login

	if err == nil {
		// Generate nonce
		nonceBytes := make([]byte, 32)
		_, _ = rand.Read(nonceBytes)
		nonce := hex.EncodeToString(nonceBytes)

		requestID, _ := r.Context().Value(contextkeys.RequestIDKey).(string)

		// Store in Redis
		nonceData := map[string]string{
			"serverLoginState": base64.RawURLEncoding.EncodeToString(serverLoginState),
			"email":            hashedEmailHex,
			"fake":             strconv.FormatBool(isFake),
			"trackM1RequestID": requestID,
		}
		nonceJSON, err := json.Marshal(nonceData)
		if err != nil {
			return apperrors.NewInternal(err)
		}

		encryptedJSON, err := crypto.EncryptRedisPayload(nonceJSON)
		if err != nil {
			return apperrors.NewInternal(err)
		}

		set := h.redis.Set(r.Context(), "login:nonce:"+nonce, encryptedJSON, 30*time.Second)
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
	const genericLoginError = "invalid email or password — if you recently signed up, check your inbox for a verification link"

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

	slog.DebugContext(r.Context(), "persistSession", m3.PersistSession)

	// Fetch from Redis
	encryptedNonceJSON, err := h.redis.Get(context.Background(), "login:nonce:"+m3.Nonce).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return apperrors.NewBadRequest(genericLoginError, err)
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
	isFake, _ := strconv.ParseBool(nonceData["fake"])
	trackM1RequestID := nonceData["trackM1RequestID"]

	if trackM1RequestID != "" {
		ctx := context.WithValue(r.Context(), contextkeys.TrackM1RequestIDKey, trackM1RequestID)
		r = r.WithContext(ctx)
	}

	finishLoginReqBytes, err := base64.RawURLEncoding.DecodeString(m3.FinishLoginRequest)
	if err != nil {
		http.Error(w, "invalid base64 in finish login request", http.StatusBadRequest)
		return apperrors.NewBadRequest("invalid base64 in finish login request", err)
	}

	// Always run FinishLogin — real computation in both paths
	// For fake sessions this will fail cryptographically, which is correct
	_, err = bindings.FinishLogin(
		serverLoginState,
		finishLoginReqBytes,
	)

	if err != nil || isFake {
		// Both wrong-password (err != nil) and fake sessions (isFake)
		// return the identical error — attacker cannot distinguish them
		slog.DebugContext(r.Context(), "bindings FinishLogin call failed: %v", err)
		return apperrors.NewUnauthorized(genericLoginError, err)
	}

	slog.DebugContext(r.Context(), "Established a new trusted session key.")

	var trustedUserInfo dto.TrustedUserInformation

	err = h.db.Model(&model.User{}).
		Select("users.*, users.encrypted_email AS email, user_key_stores.*").
		Joins("LEFT JOIN user_key_stores ON user_key_stores.user_id = users.id").
		Where("users.hashed_email = ?", hashedEmail).
		First(&trustedUserInfo).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperrors.NewNotFound(genericLoginError, err)
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

	const persistedRefreshTokenLifetime = 7 * 24 * time.Hour // default 7 days
	const transientRefreshTokenLifetime = 24 * time.Hour     // default 24 hours

	var refreshTokenLifetime time.Duration
	if m3.PersistSession == true {
		refreshTokenLifetime = persistedRefreshTokenLifetime
	} else {
		refreshTokenLifetime = transientRefreshTokenLifetime
	}

	expiresAt := time.Now().Add(refreshTokenLifetime)

	refreshTokenEntry := model.GormRefreshToken{
		UserID:        trustedUserInfo.ID,
		TokenHash:     refreshToken.TokenSha256Hash,
		FamilyID:      familyID,
		IsRevoked:     false,
		ExpiresAt:     expiresAt,
		CsrfTokenHash: newCsrfToken.TokenSha256Hash,
		Session:       newSessionEntry,
		DpopJKT:       thumbprint,
	}

	fingerprintBytes := make([]byte, 64)
	_, _ = rand.Read(fingerprintBytes)
	fingerprint := hex.EncodeToString(fingerprintBytes)

	fingerprintHashBytes := sha256.Sum256(fingerprintBytes)
	fingerprintHash := hex.EncodeToString(fingerprintHashBytes[:])

	accessToken, expTime := token.IssueAccessToken(false, fingerprintHash, thumbprint, trustedUserInfo.ID.String(), trustedUserInfo.Email, generatedNewSessionID.String(), familyID.String())
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

	if m3.PersistSession == true {
		http.SetCookie(w, &http.Cookie{
			Name:     "__Secure-Auth",
			Value:    refreshToken.Token,
			HttpOnly: true,
			Path:     "/",
			Secure:   true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   int(persistedRefreshTokenLifetime / time.Second),
			Expires:  time.Now().Add(persistedRefreshTokenLifetime),
		})
	} else {
		http.SetCookie(w, &http.Cookie{
			Name:     "__Secure-Auth",
			Value:    refreshToken.Token,
			HttpOnly: true,
			Path:     "/",
			Secure:   true,
			SameSite: http.SameSiteLaxMode,
			//don't set MaxAge or Expires
		})
	}

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
