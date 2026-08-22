package account

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"quartz/config"
	"quartz/internal/bindings"
	"quartz/internal/dto"
	"quartz/internal/model"
	apperrors "quartz/pkg/app-errors"
	"quartz/pkg/captcha"
	"quartz/pkg/crypto"
	"quartz/pkg/email"
	"time"

	"crypto/sha256"
	"encoding/hex"

	"github.com/go-redis/redis_rate/v10"
	"github.com/google/uuid"
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

type ForgotPasswordRequest struct {
	Email          string `json:"email"`
	TurnstileToken string `json:"turnstileToken"`
}

func (h *Handler) ForgotPassword(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodPost {
		return apperrors.NewMethodNotAllowed("method not allowed")
	}

	var req ForgotPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return apperrors.NewBadRequest("Request body is invalid", err)
	}

	if req.TurnstileToken == "" {
		slog.Debug("no turnstile token in request body")
		return apperrors.NewBadRequest("no turntile token in request", fmt.Errorf("no turnstile token in request"))
	}

	cfip := r.Header.Get("CF-Connecting-IP")
	if cfip == "" {
		cfip = r.Header.Get("X-Forwarded-For")
	}
	if cfip == "" {
		cfip = r.Header.Get("X-Real-IP")
	}

	success, errorsList, err := captcha.VerifyTurnstileToken(req.TurnstileToken, cfip)
	if err != nil {
		log.Printf("Turnstile verification failed: %v", err)
		return apperrors.NewBadRequest("invalid turnstile token", err)
	}
	if !success {
		log.Printf("Turnstile verification failed: %v", errorsList)
		return apperrors.NewBadRequest("invalid turnstile token", nil)
	}

	emailHashBytes := sha256.Sum256([]byte(req.Email))
	emailHash := hex.EncodeToString(emailHashBytes[:])

	limiter := redis_rate.NewLimiter(h.redis)

	// 3 per 24 hours
	res24h, err := limiter.Allow(r.Context(), "password_reset:24h:"+emailHash, redis_rate.Limit{Rate: 3, Burst: 3, Period: 24 * time.Hour})
	if err == nil && res24h.Allowed == 0 {
		log.Printf("Rate limit (24h) exceeded for forgot password: %s", req.Email)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		return nil
	}

	// 1 per 1 minute
	res1m, err := limiter.Allow(r.Context(), "password_reset:1m:"+emailHash, redis_rate.Limit{Rate: 1, Burst: 1, Period: time.Minute})
	if err == nil && res1m.Allowed == 0 {
		log.Printf("Rate limit (1m) exceeded for forgot password: %s", req.Email)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		return nil
	}

	var user model.User
	err = h.db.Where("email = ?", req.Email).First(&user).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return apperrors.NewInternal(err)
	}
	if err != nil && errors.Is(err, gorm.ErrRecordNotFound) {
		// Don't leak whether user exists, we won't send an email either way
		log.Printf("ForgotPasswordRequest: email not found, silently failing")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		return nil
	}

	// Generate secure token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return apperrors.NewInternal(err)
	}

	tokenString := fmt.Sprintf("%x", tokenBytes)

	tokenHashBytes := sha256.Sum256([]byte(tokenString))
	tokenHash := fmt.Sprintf("%x", tokenHashBytes)

	token := model.PasswordResetToken{
		UserID:    user.ID,
		TokenHash: tokenHash,
		ExpiresAt: time.Now().Add(15 * time.Minute),
	}

	if err := h.db.Create(&token).Error; err != nil {
		return apperrors.NewInternal(err)
	}

	magicLink := fmt.Sprintf("%s/reset-password?token=%s", config.Cfg.App.FrontendURL, tokenString)

	// Send email using Resend
	if err := email.SendPasswordReset(r.Context(), req.Email, magicLink); err != nil {
		log.Printf("Failed to send email: %v", err)

		// Fallback for local development if Resend isn't configured yet
		if config.Cfg.Env == "development" {
			fmt.Printf("LOCAL DEV MAGIC LINK FOR %s: %s\n", req.Email, magicLink)
			// Continue returning 200 OK so the dev can copy the link from the terminal
		} else {
			return apperrors.NewInternal(err)
		}
	}

	w.WriteHeader(http.StatusOK)
	err = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	if err != nil {
		return apperrors.NewInternal(err)
	}
	return nil
}

type VerifyCodeRequest struct {
	Token string `json:"token"`
}

type VerifyCodeResponse struct {
	Status       string      `json:"status"`
	Email        string      `json:"email"`
	RecoveryKeys dto.KeysDTO `json:"recoveryKeys"`
}

func (h *Handler) VerifyResetCode(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodPost {
		return apperrors.NewMethodNotAllowed("method not allowed")
	}

	var req VerifyCodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return apperrors.NewBadRequest("Request body is invalid", err)
	}

	tokenHashBytes := sha256.Sum256([]byte(req.Token))
	tokenHash := fmt.Sprintf("%x", tokenHashBytes)

	var token model.PasswordResetToken
	if err := h.db.Preload("User").Where("token_hash = ? AND expires_at > ?", tokenHash, time.Now()).First(&token).Error; err != nil {
		return apperrors.NewBadRequest("invalid or expired token", err)
	}

	var keyStore model.UserKeyStore
	if err := h.db.Where("user_id = ?", token.UserID).First(&keyStore).Error; err != nil {
		return apperrors.NewInternal(err)
	}

	res := VerifyCodeResponse{
		Status: "ok",
		Email:  token.User.Email,
		RecoveryKeys: dto.KeysDTO{
			MasterKdfSalt:              keyStore.MasterKdfSalt,
			AccountEncryptionPublicKey: keyStore.AccountEncryptionPublicKey,
			AccountSigningPublicKey:    keyStore.AccountSigningPublicKey,

			RecoveryEncryptedAccountEncryptionPrivateKey: keyStore.RecoveryEncryptedAccountEncryptionPrivateKey,
			RecoveryAccountEncryptionKeyNonce:            keyStore.RecoveryAccountEncryptionKeyNonce,
			RecoveryEncryptedAccountSigningPrivateKey:    keyStore.RecoveryEncryptedAccountSigningPrivateKey,
			RecoveryAccountSigningKeyNonce:               keyStore.RecoveryAccountSigningKeyNonce,
		},
	}

	w.WriteHeader(http.StatusOK)
	err := json.NewEncoder(w).Encode(res)
	if err != nil {
		return apperrors.NewInternal(err)
	}
	return nil
}

type ResetM1 struct {
	Email               string `json:"email"`
	Token               string `json:"token"`
	RegistrationRequest string `json:"registrationRequest"`
}

func (h *Handler) ResetPasswordM1(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodPost {
		return apperrors.NewMethodNotAllowed("method not allowed")
	}

	var m1 ResetM1
	if err := json.NewDecoder(r.Body).Decode(&m1); err != nil {
		return apperrors.NewBadRequest("Request body is invalid", err)
	}

	var user model.User
	if err := h.db.Where("email = ?", m1.Email).First(&user).Error; err != nil {
		return apperrors.NewBadRequest("invalid request", err)
	}

	var token model.PasswordResetToken
	if err := h.db.Where("user_id = ? AND token = ? AND expires_at > ?", user.ID, m1.Token, time.Now()).First(&token).Error; err != nil {
		return apperrors.NewBadRequest("invalid or expired token", err)
	}

	regReqBytes, err := base64.RawURLEncoding.DecodeString(m1.RegistrationRequest)
	if err != nil {
		return apperrors.NewBadRequest("invalid base64 in registration request", err)
	}

	// We reuse the existing user ID as the credential ID
	regResponse, err := bindings.StartRegistration(
		h.opaqueSetup,
		regReqBytes,
		[]byte(user.ID.String()),
	)
	if err != nil {
		return apperrors.NewBadRequest("registration failed", err)
	}

	// Store the new nonce temporarily in redis
	newNonce := uuid.New().String()
	encryptedNonceData, err := crypto.EncryptRedisPayload([]byte(user.ID.String()))
	if err != nil {
		return apperrors.NewInternal(err)
	}

	err = h.redis.Set(context.Background(), "reset:nonce:"+newNonce, encryptedNonceData, 15*time.Minute).Err()
	if err != nil {
		return apperrors.NewInternal(err)
	}

	w.WriteHeader(http.StatusOK)
	err = json.NewEncoder(w).Encode(dto.M2{
		Status:               "ok",
		RegistrationResponse: base64.RawURLEncoding.EncodeToString(regResponse),
		Nonce:                newNonce,
	})
	if err != nil {
		return apperrors.NewInternal(err)
	}
	return nil
}

type ResetM3 struct {
	Email string      `json:"email"`
	Token string      `json:"token"`
	User  dto.UserDTO `json:"user"`
}

func (h *Handler) ResetPasswordM3(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodPost {
		return apperrors.NewMethodNotAllowed("Provided method is not allowed")
	}

	var m3 ResetM3
	if err := json.NewDecoder(r.Body).Decode(&m3); err != nil {
		return apperrors.NewBadRequest("Provided request body is invalid", err)
	}

	var user model.User
	if err := h.db.Where("email = ?", m3.Email).First(&user).Error; err != nil {
		return apperrors.NewBadRequest("invalid request", err)
	}

	var token model.PasswordResetToken
	if err := h.db.Where("user_id = ? AND token = ? AND expires_at > ?", user.ID, m3.Token, time.Now()).First(&token).Error; err != nil {
		return apperrors.NewBadRequest("invalid or expired token", err)
	}

	// Verify the nonce
	storedEncryptedUserID, err := h.redis.Get(context.Background(), "reset:nonce:"+m3.User.APAKE.RegistrationNonce).Result()
	if err != nil {
		return apperrors.NewBadRequest("invalid registration nonce", err)
	}

	storedUserIDBytes, err := crypto.DecryptRedisPayload(storedEncryptedUserID)
	if err != nil || string(storedUserIDBytes) != user.ID.String() {
		return apperrors.NewBadRequest("invalid registration nonce", err)
	}

	regRecordBytes, err := base64.RawURLEncoding.DecodeString(m3.User.APAKE.RegistrationRecord)
	if err != nil {
		return apperrors.NewBadRequest("invalid base64 in registration record", err)
	}

	passwordFileRecord, err := bindings.FinishRegistration(
		regRecordBytes,
	)

	if err != nil {
		return apperrors.NewInternal(err)
	}

	err = h.db.Transaction(func(tx *gorm.DB) error {
		// Update user registration record
		if err := tx.Model(&user).Updates(map[string]interface{}{
			"registration_record": base64.RawURLEncoding.EncodeToString(passwordFileRecord),
			"registration_nonce":  m3.User.APAKE.RegistrationNonce,
		}).Error; err != nil {
			return err
		}

		// Update keystore with newly encrypted standard keys
		// Recovery keys remain the same (unless frontend re-encrypts them too)
		if err := tx.Model(&model.UserKeyStore{}).Where("user_id = ?", user.ID).Updates(map[string]interface{}{
			"master_kdf_salt":                          m3.User.Keys.MasterKdfSalt,
			"encrypted_account_encryption_private_key": m3.User.Keys.EncryptedAccountEncryptionPrivateKey,
			"account_encryption_key_nonce":             m3.User.Keys.AccountEncryptionKeyNonce,
			"encrypted_account_signing_private_key":    m3.User.Keys.EncryptedAccountSigningPrivateKey,
			"account_signing_key_nonce":                m3.User.Keys.AccountSigningKeyNonce,

			"recovery_encrypted_account_encryption_private_key": m3.User.Keys.RecoveryEncryptedAccountEncryptionPrivateKey,
			"recovery_account_encryption_key_nonce":             m3.User.Keys.RecoveryAccountEncryptionKeyNonce,
			"recovery_encrypted_account_signing_private_key":    m3.User.Keys.RecoveryEncryptedAccountSigningPrivateKey,
			"recovery_account_signing_key_nonce":                m3.User.Keys.RecoveryAccountSigningKeyNonce,
		}).Error; err != nil {
			return err
		}

		// Delete the token so it can't be reused
		if err := tx.Delete(&token).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return apperrors.NewInternal(err)
	}

	// Clean up redis
	h.redis.Del(context.Background(), "reset:nonce:"+m3.User.APAKE.RegistrationNonce)

	w.WriteHeader(http.StatusOK)
	err = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	if err != nil {
		return apperrors.NewInternal(err)
	}
	return nil
}
