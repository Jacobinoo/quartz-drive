package account

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"quartz/config"
	"quartz/internal/bindings"
	"quartz/internal/dto"
	"quartz/internal/model"
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

func (h *Handler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ForgotPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var user model.User
	if err := h.db.Where("email = ?", req.Email).First(&user).Error; err != nil {
		// Don't leak whether user exists
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		return
	}

	// Generate 6 digit code
	max := big.NewInt(1000000)
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		http.Error(w, "failed to generate token", http.StatusInternalServerError)
		return
	}
	code := fmt.Sprintf("%06d", n.Int64())

	token := model.PasswordResetToken{
		UserID:    user.ID,
		Token:     code,
		ExpiresAt: time.Now().Add(15 * time.Minute),
	}

	if err := h.db.Create(&token).Error; err != nil {
		http.Error(w, "failed to create token", http.StatusInternalServerError)
		return
	}

	// Send email using Resend
	client := resend.NewClient(config.Cfg.Email.Key)

	params := &resend.SendEmailRequest{
		From:    "Quartz <noreply@resend.dev>", // Replace with verified domain in production
		To:      []string{req.Email},
		Subject: "Quartz Account Recovery Code",
		Html:    fmt.Sprintf("<p>Your Quartz account recovery code is: <strong>%s</strong></p><p>This code expires in 15 minutes.</p>", code),
	}

	_, err = client.Emails.Send(params)
	if err != nil {
		log.Printf("Failed to send email: %v", err)
		// For local testing without a verified domain/key, we print the code
		fmt.Printf("LOCAL DEV RECOVERY CODE FOR %s: %s\n", req.Email, code)
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

type VerifyCodeRequest struct {
	Email string `json:"email"`
	Code  string `json:"code"`
}

type VerifyCodeResponse struct {
	Status string `json:"status"`
	RecoveryKeys dto.KeysDTO `json:"recoveryKeys"`
}

func (h *Handler) VerifyResetCode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req VerifyCodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var user model.User
	if err := h.db.Where("email = ?", req.Email).First(&user).Error; err != nil {
		http.Error(w, "invalid code or email", http.StatusBadRequest)
		return
	}

	var token model.PasswordResetToken
	if err := h.db.Where("user_id = ? AND token = ? AND expires_at > ?", user.ID, req.Code, time.Now()).First(&token).Error; err != nil {
		http.Error(w, "invalid or expired code", http.StatusBadRequest)
		return
	}

	var keyStore model.UserKeyStore
	if err := h.db.Where("user_id = ?", user.ID).First(&keyStore).Error; err != nil {
		http.Error(w, "user keystore not found", http.StatusInternalServerError)
		return
	}

	res := VerifyCodeResponse{
		Status: "ok",
		RecoveryKeys: dto.KeysDTO{
			MasterKdfSalt: keyStore.MasterKdfSalt,
			AccountEncryptionPublicKey: keyStore.AccountEncryptionPublicKey,
			AccountSigningPublicKey: keyStore.AccountSigningPublicKey,
			
			RecoveryEncryptedAccountEncryptionPrivateKey: keyStore.RecoveryEncryptedAccountEncryptionPrivateKey,
			RecoveryAccountEncryptionKeyNonce:            keyStore.RecoveryAccountEncryptionKeyNonce,
			RecoveryEncryptedAccountSigningPrivateKey:    keyStore.RecoveryEncryptedAccountSigningPrivateKey,
			RecoveryAccountSigningKeyNonce:               keyStore.RecoveryAccountSigningKeyNonce,
		},
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(res)
}

type ResetM1 struct {
	Email               string `json:"email"`
	Code                string `json:"code"`
	RegistrationRequest string `json:"registrationRequest"`
}

func (h *Handler) ResetPasswordM1(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var m1 ResetM1
	if err := json.NewDecoder(r.Body).Decode(&m1); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var user model.User
	if err := h.db.Where("email = ?", m1.Email).First(&user).Error; err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	var token model.PasswordResetToken
	if err := h.db.Where("user_id = ? AND token = ? AND expires_at > ?", user.ID, m1.Code, time.Now()).First(&token).Error; err != nil {
		http.Error(w, "invalid or expired code", http.StatusBadRequest)
		return
	}

	regReqBytes, err := base64.RawURLEncoding.DecodeString(m1.RegistrationRequest)
	if err != nil {
		http.Error(w, "invalid base64 in registration request", http.StatusBadRequest)
		return
	}

	// We reuse the existing user ID as the credential ID
	regResponse, err := bindings.StartRegistration(
		h.opaqueSetup,
		regReqBytes,
		[]byte(user.ID.String()),
	)

	if err != nil {
		http.Error(w, "registration failed", http.StatusBadRequest)
		return
	}

	// Store the new nonce temporarily in redis
	newNonce := uuid.New().String()
	err = h.redis.Set(context.Background(), "reset:nonce:"+newNonce, user.ID.String(), 15*time.Minute).Err()
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(dto.M2{
		Status:               "ok",
		RegistrationResponse: base64.RawURLEncoding.EncodeToString(regResponse),
		Nonce:                newNonce,
	})
}

type ResetM3 struct {
	Email string `json:"email"`
	Code  string `json:"code"`
	User  dto.UserDTO `json:"user"`
}

func (h *Handler) ResetPasswordM3(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var m3 ResetM3
	if err := json.NewDecoder(r.Body).Decode(&m3); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var user model.User
	if err := h.db.Where("email = ?", m3.Email).First(&user).Error; err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	var token model.PasswordResetToken
	if err := h.db.Where("user_id = ? AND token = ? AND expires_at > ?", user.ID, m3.Code, time.Now()).First(&token).Error; err != nil {
		http.Error(w, "invalid or expired code", http.StatusBadRequest)
		return
	}

	// Verify the nonce
	storedUserID, err := h.redis.Get(context.Background(), "reset:nonce:"+m3.User.APAKE.RegistrationNonce).Result()
	if err != nil || storedUserID != user.ID.String() {
		http.Error(w, "invalid registration nonce", http.StatusBadRequest)
		return
	}

	regRecordBytes, err := base64.RawURLEncoding.DecodeString(m3.User.APAKE.RegistrationRecord)
	if err != nil {
		http.Error(w, "invalid base64 in registration record", http.StatusBadRequest)
		return
	}

	passwordFileRecord, err := bindings.FinishRegistration(
		regRecordBytes,
	)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
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
			"master_kdf_salt": m3.User.Keys.MasterKdfSalt,
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Clean up redis
	h.redis.Del(context.Background(), "reset:nonce:"+m3.User.APAKE.RegistrationNonce)

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
