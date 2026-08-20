package account

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"quartz/config"
	"quartz/internal/bindings"
	"quartz/internal/dto"
	"quartz/internal/model"
	apperrors "quartz/pkg/app-errors"
	"time"

	"github.com/resend/resend-go/v2"

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
	Email string `json:"email"`
}

func (h *Handler) ForgotPassword(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodPost {
		return apperrors.NewMethodNotAllowed("method not allowed")
	}

	var req ForgotPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return apperrors.NewBadRequest("Request body is invalid", err)
	}

	var user model.User
	err := h.db.Where("email = ?", req.Email).First(&user).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return apperrors.NewInternal(err)
	}
	if err != nil && errors.Is(err, gorm.ErrRecordNotFound) {
		// Don't leak whether user exists, we won't send an email either way
		log.Printf("ForgotPasswordRequest: email not found, silently failing")
		w.WriteHeader(http.StatusOK)
		err := json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		if err != nil {
			return apperrors.NewInternal(err)
		}
		return nil
	}

	// Generate secure token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return apperrors.NewInternal(err)
	}
	tokenStr := fmt.Sprintf("%x", tokenBytes)

	token := model.PasswordResetToken{
		UserID:    user.ID,
		Token:     tokenStr,
		ExpiresAt: time.Now().Add(15 * time.Minute),
	}

	if err := h.db.Create(&token).Error; err != nil {
		return apperrors.NewInternal(err)
	}

	// Send email using Resend
	client := resend.NewClient(config.Cfg.Email.Key)

	magicLink := fmt.Sprintf("%s/reset-password?token=%s&email=%s", config.Cfg.App.FrontendURL, tokenStr, req.Email)

	from := fmt.Sprintf("%s <%s>", config.Cfg.Email.UpdatesFromSenderName, config.Cfg.Email.UpdatesVerifiedDomain)

	params := &resend.SendEmailRequest{
		From:    from,
		To:      []string{req.Email},
		Subject: "Quartz Account Recovery",
		Html:    fmt.Sprintf("<p>Click the link below to recover your Quartz account:</p><p><a href=\"%s\">Recover Account</a></p><p>This link expires in 15 minutes.</p>", magicLink),
	}

	//should this be sent asynchronously? are we blocking the request?
	// TODO: support idempotency keys to avoid duplicates
	opt := &resend.SendEmailOptions{
		//IdempotencyKey: "",
	}
	// TODO: is r.Context() here correct?
	_, err = client.Emails.SendWithOptions(r.Context(), params, opt)
	if err != nil {
		log.Printf("Failed to send email: %v", err)
		// For local testing without a verified domain/key, we print the link
		fmt.Printf("LOCAL DEV MAGIC LINK FOR %s: %s\n", req.Email, magicLink)
	}

	w.WriteHeader(http.StatusOK)
	err = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	if err != nil {
		return apperrors.NewInternal(err)
	}
	return nil
}

type VerifyCodeRequest struct {
	Email string `json:"email"`
	Token string `json:"token"`
}

type VerifyCodeResponse struct {
	Status       string      `json:"status"`
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

	var user model.User
	err := h.db.Where("email = ?", req.Email).First(&user).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return apperrors.NewInternal(err)
	}
	if err != nil && errors.Is(err, gorm.ErrRecordNotFound) {
		// Don't leak whether user exists, respond with a fake error (token invalid instead of email not found)
		log.Printf("VerifyCodeRequest: invalid token or email, failing with a fake error")
		return apperrors.NewBadRequest("invalid or expired token", err)
	}

	var token model.PasswordResetToken
	if err := h.db.Where("user_id = ? AND token = ? AND expires_at > ?", user.ID, req.Token, time.Now()).First(&token).Error; err != nil {
		return apperrors.NewBadRequest("invalid or expired token", err)
	}

	var keyStore model.UserKeyStore
	if err := h.db.Where("user_id = ?", user.ID).First(&keyStore).Error; err != nil {
		return apperrors.NewInternal(err)
	}

	res := VerifyCodeResponse{
		Status: "ok",
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
	err = json.NewEncoder(w).Encode(res)
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
	err = h.redis.Set(context.Background(), "reset:nonce:"+newNonce, user.ID.String(), 15*time.Minute).Err()
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
	storedUserID, err := h.redis.Get(context.Background(), "reset:nonce:"+m3.User.APAKE.RegistrationNonce).Result()
	if err != nil || storedUserID != user.ID.String() {
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
