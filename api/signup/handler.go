package signup

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"quartz/internal/bindings"
	"quartz/internal/dto"
	apperrors "quartz/pkg/app-errors"
	"quartz/pkg/captcha"
	"quartz/pkg/contextkeys"
	crypto "quartz/pkg/crypto"
	"quartz/pkg/email"
	"quartz/pkg/worker"
	"strings"
	"time"

	// pb "quartz/proto"

	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Handler struct {
	db          *gorm.DB
	redis       *redis.Client
	opaqueSetup []byte
	asynqClient *asynq.Client
}

func NewHandler(db *gorm.DB, redisClient *redis.Client, opaqueSetup []byte, asynqClient *asynq.Client) *Handler {
	return &Handler{db: db, redis: redisClient, opaqueSetup: opaqueSetup, asynqClient: asynqClient}
}

type redisRegistrationSession struct {
	HashedEmailHex string `json:"hashed_email"`
	UserID         string `json:"user_id"`
}

// Signup: (opaque receive m1 & send m2)
func (h *Handler) Signup(w http.ResponseWriter, r *http.Request) error {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		return apperrors.NewMethodNotAllowed("method not allowed")
	}

	var m1 dto.M1
	if err := json.NewDecoder(r.Body).Decode(&m1); err != nil {
		return apperrors.NewBadRequest("invalid request", err)
	}

	if m1.RegistrationRequest == "" || m1.Email == "" {
		return apperrors.NewBadRequest("some fields are missing", fmt.Errorf("some fields are missing"))
	}

	regReqBytes, err := base64.RawURLEncoding.DecodeString(m1.RegistrationRequest)
	if err != nil {
		slog.WarnContext(r.Context(), "registration request base64 decode error",
			"error", err,
		)
		return apperrors.NewBadRequest("invalid registration request", err)
	}
	if len(regReqBytes) != crypto.OpaqueRegistrationRequestSize {
		slog.WarnContext(r.Context(), "registration request base64 length mismatch",
			"expected", crypto.OpaqueRegistrationRequestSize,
			"got", len(regReqBytes),
		)
		return apperrors.NewBadRequest("invalid registration request size", nil)
	}

	emailTrustScore := email.ValidateAndParseEmail(m1.Email)

	if emailTrustScore <= 0 {
		slog.WarnContext(r.Context(), "untrusted email detected, possible abuse, temporary email or bot", "`email`", m1.Email)
		return apperrors.NewBadRequest("email is invalid", nil)
	} else if emailTrustScore <= 0.6 {
		slog.WarnContext(r.Context(), "email from untrusted provider detected, logging for investigation", "`email`", m1.Email)
	}

	success, errArr, err := captcha.VerifyCaptchaTokenInRequest(r)
	if err != nil {
		slog.WarnContext(r.Context(), "captcha verification failed", "errors", strings.Join(errArr, ","), "error", err)
		return apperrors.NewBadRequest("invalid token", err)
	}
	if !success {
		slog.WarnContext(r.Context(), "captcha verification failed", "errors", strings.Join(errArr, ","))
		return apperrors.NewBadRequest("invalid token", nil)
	}

	hashedEmail, err := crypto.HashEmail([]byte(m1.Email))
	if err != nil {
		return apperrors.NewInternal(err)
	}
	hashedEmailHex := fmt.Sprintf("%x", hashedEmail)

	userId := uuid.New().String()

	sessionNonceBytes := make([]byte, 32)
	rand.Read(sessionNonceBytes)
	sessionNonceB64 := base64.RawURLEncoding.EncodeToString(sessionNonceBytes)

	sessionRedisKey := fmt.Sprintf("reg_session:%s", sessionNonceB64)
	sessionRedisPayload, _ := json.Marshal(redisRegistrationSession{
		HashedEmailHex: hashedEmailHex,
		UserID:         userId,
	})

	encryptedRedisPayload, err := crypto.EncryptRedisPayload(sessionRedisPayload)
	if err != nil {
		return apperrors.NewInternal(err)
	}

	const sessionRedisTTL = 30 * time.Second
	redisCmdErr := h.redis.Set(r.Context(), sessionRedisKey, encryptedRedisPayload, sessionRedisTTL).Err()
	if redisCmdErr != nil {
		return apperrors.NewInternal(redisCmdErr)
	}

	var registrationResponse dto.M2

	regResponse, err := bindings.StartRegistration(
		h.opaqueSetup,
		regReqBytes,
		[]byte(userId),
	)
	if err != nil {
		return apperrors.NewInternal(err)
	}

	registrationResponse = dto.M2{
		Status:               "ok",
		RegistrationResponse: base64.RawURLEncoding.EncodeToString(regResponse),
		Nonce:                sessionNonceB64,
	}

	slog.InfoContext(r.Context(), "user wants to register, m1 correct, waiting for m3")

	w.WriteHeader(http.StatusOK)
	err = json.NewEncoder(w).Encode(registrationResponse)
	if err != nil {
		return apperrors.NewInternal(err)
	}
	return nil
}

// base64RawUrlDecodedByteLengthCompareWith compares the decoded base 64 length with compareWith parameter.
// Returns nil if comparison successful, otherwise error.
func base64RawUrlDecodedByteLengthCompareWith(b64 string, compareWith int) error {
	decoded, err := base64.RawURLEncoding.DecodeString(b64)
	if err != nil {
		return fmt.Errorf("base64 decoding failed")
	}
	if len(decoded) != compareWith {
		return fmt.Errorf("base64 decoded length mismatch, got %d, expected %d", len(decoded), compareWith)
	}
	return nil
}

func validateM3Payload(m3 *dto.M3) error {
	if m3.User.Email == "" {
		return fmt.Errorf("m3 is missing email")
	}

	checks := []struct {
		value    string
		expected int
	}{
		{m3.User.APAKE.RegistrationNonce, crypto.RegistrationNonceBytes},
		{m3.User.APAKE.RegistrationRecord, crypto.OpaqueRegistrationRecordBytes},

		{m3.User.Keys.MasterKdfSalt, crypto.MasterKdfSaltBytes},
		{m3.User.Keys.AccountEncryptionPublicKey, crypto.EncryptionPublicKeyBytes},
		{m3.User.Keys.EncryptedAccountEncryptionPrivateKey, crypto.EncryptedAccountEncryptionPrivateKeyBytes},
		{m3.User.Keys.AccountEncryptionKeyNonce, crypto.SodiumNonceBytes},
		{m3.User.Keys.AccountSigningPublicKey, crypto.AccountSigningPublicKeyBytes},
		{m3.User.Keys.EncryptedAccountSigningPrivateKey, crypto.EncryptedAccountSigningPrivateKeyBytes},
		{m3.User.Keys.AccountSigningKeyNonce, crypto.SodiumNonceBytes},
		{m3.User.Keys.RecoveryEncryptedAccountEncryptionPrivateKey, crypto.RecoveryEncryptedAccountEncryptionPrivateKeyBytes},
		{m3.User.Keys.RecoveryAccountEncryptionKeyNonce, crypto.SodiumNonceBytes},
		{m3.User.Keys.RecoveryEncryptedAccountSigningPrivateKey, crypto.RecoveryEncryptedAccountSigningPrivateKeyBytes},
		{m3.User.Keys.RecoveryAccountSigningKeyNonce, crypto.SodiumNonceBytes},

		{m3.Drive.DefaultShare.PublicKey, crypto.SharePublicKeyBytes},
		{m3.Drive.DefaultShare.WrappedPrivateKey, crypto.ShareWrappedPrivateKeyBytes},
		{m3.Drive.DefaultShare.PrivKeyNonce, crypto.SodiumNonceBytes},
		{m3.Drive.DefaultShare.EncryptedPassphraseForOwner, crypto.ShareEncryptedPassphraseForOwnerBytes},
		{m3.Drive.DefaultShare.SignedEncryptedPassphraseForOwner, crypto.ShareSignedEncryptedPassphraseForOwnerBytes},
		{m3.Drive.RootNode.PublicKey, crypto.NodePublicKeyBytes},
		{m3.Drive.RootNode.WrappedPrivateKey, crypto.NodeWrappedPrivateKeyBytes},
		{m3.Drive.RootNode.PrivKeyNonce, crypto.SodiumNonceBytes},
		{m3.Drive.RootNode.EncryptedPassphrase, crypto.NodeEncryptedPassphraseBytes},
		{m3.Drive.RootNode.SignedEncryptedPassphrase, crypto.NodeSignedEncryptedPassphraseBytes},
	}

	for i, check := range checks {
		if err := base64RawUrlDecodedByteLengthCompareWith(check.value, check.expected); err != nil {
			return fmt.Errorf("m3 validation error: %v. check %d failed`", err, i)
		}
	}
	return nil
}

func (h *Handler) SignupM3(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodPost {
		return apperrors.NewMethodNotAllowed("method not allowed")
	}

	var m3 dto.M3

	if err := json.NewDecoder(r.Body).Decode(&m3); err != nil {
		slog.WarnContext(r.Context(), "decode m3 payload error", "error", err)
		return apperrors.NewBadRequest("invalid request", err)
	}

	err := validateM3Payload(&m3)
	if err != nil {
		slog.WarnContext(r.Context(), "validate m3 payload error", "error", err)
		return apperrors.NewBadRequest("invalid request", err)
	}

	sessionRedisKey := fmt.Sprintf("reg_session:%s", m3.User.APAKE.RegistrationNonce)
	sessionRedisCmd := h.redis.GetDel(r.Context(), sessionRedisKey) // atomic get+delete = one-time use
	err = sessionRedisCmd.Err()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return apperrors.NewBadRequest("invalid or expired registration session", nil)
		}
		return apperrors.NewInternal(err)
	}
	sessionRedisCmdVal := sessionRedisCmd.Val()

	decryptedRedisPayloadBytes, err := crypto.DecryptRedisPayload(sessionRedisCmdVal)
	if err != nil {
		return apperrors.NewInternal(err)
	}

	var registrationSession redisRegistrationSession
	err = json.Unmarshal(decryptedRedisPayloadBytes, &registrationSession)
	if err != nil {
		return apperrors.NewInternal(err)
	}

	hashedIncomingEmail, err := crypto.HashEmail([]byte(m3.User.Email))
	if err != nil {
		return apperrors.NewInternal(err)
	}
	hashedIncomingEmailHex := hex.EncodeToString([]byte(hashedIncomingEmail))

	if subtle.ConstantTimeCompare([]byte(hashedIncomingEmailHex), []byte(registrationSession.HashedEmailHex)) == 0 {
		slog.WarnContext(r.Context(), "email mismatch", "hashed_incoming_email_hex", hashedIncomingEmailHex, "session_saved_hashed_email_hex", registrationSession.HashedEmailHex)
		return apperrors.NewBadRequest("invalid or expired registration session", nil)
	}

	userID, err := uuid.Parse(registrationSession.UserID)
	if err != nil {
		return apperrors.NewInternal(err)
	}

	regRecordBytes, _ := base64.RawURLEncoding.DecodeString(m3.User.APAKE.RegistrationRecord)
	passwordFileRecord, err := bindings.FinishRegistration(
		regRecordBytes,
	)
	if err != nil {
		return apperrors.NewInternal(err)
	}

	// Handle database lookups & inserting + sending emails in the background worker
	// This also prevents timing attacks to enumerate the emails registered in our system

	reqIDStr, ok := r.Context().Value(contextkeys.RequestIDKey).(string)
	if !ok {
		return apperrors.NewInternal(fmt.Errorf("request ID not found in context"))
	}

	task, err := worker.NewSignupProcessingTask(reqIDStr, m3.User.Email, registrationSession.HashedEmailHex, userID, passwordFileRecord, &m3)
	if err != nil {
		return apperrors.NewInternal(err)
	}

	taskTimeLimit := 10 * time.Second
	enqueuedTaskInfo, err := h.asynqClient.Enqueue(task, asynq.Timeout(taskTimeLimit), asynq.MaxRetry(3))
	if err != nil {
		return apperrors.NewInternal(err)
	}

	slog.InfoContext(r.Context(), "enqueued a signup processing task", "task_id", enqueuedTaskInfo.ID, "timeout", taskTimeLimit)

	w.WriteHeader(http.StatusAccepted)
	err = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	if err != nil {
		return apperrors.NewInternal(err)
	}
	return nil
}
