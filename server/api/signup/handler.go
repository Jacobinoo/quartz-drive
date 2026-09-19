package signup

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"quartz/config"
	"quartz/internal/bindings"
	"quartz/internal/dto"
	"quartz/internal/utils"
	apperrors "quartz/pkg/app-errors"
	"quartz/pkg/captcha"
	"quartz/pkg/contextkeys"
	crypto "quartz/pkg/crypto"
	"quartz/pkg/email"
	"quartz/pkg/worker"
	"strings"
	"time"

	// pb "quartz/proto"

	"github.com/go-redis/redis_rate/v10"
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
	HashedEmailHex   string `json:"hashed_email"`
	UserID           string `json:"user_id"`
	TrackM1RequestID string `json:"track_m1_request_id"`
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

	m1.Email = strings.ToLower(m1.Email)

	hashedEmailHex, err := crypto.HashEmail([]byte(m1.Email))
	if err != nil {
		return apperrors.NewInternal(err)
	}

	limiter := redis_rate.NewLimiter(h.redis)

	// 1 per 1 minute
	res1m, err := limiter.Allow(r.Context(), "signup:1m:"+hashedEmailHex, redis_rate.Limit{Rate: 1, Burst: 1, Period: time.Minute})
	if err != nil {
		slog.Error("rate limiter error", "err", err)
	} else if res1m.Allowed == 0 {
		slog.Debug("Rate limit (1m) exceeded for signup.", "emailHash", hashedEmailHex)
		return apperrors.NewRateLimited("You are being ratelimited. Try again in a minute.")
	}

	// 3 per 24 hours
	res24h, err := limiter.Allow(r.Context(), "signup:24h:"+hashedEmailHex, redis_rate.Limit{Rate: 3, Burst: 3, Period: 24 * time.Hour})
	if err != nil {
		slog.Error("rate limiter error", "err", err)
	} else if res24h.Allowed == 0 {
		slog.Debug("Rate limit (24h) exceeded for signup.", "emailHash", hashedEmailHex)
		return apperrors.NewRateLimited("You are being ratelimited. Try again in a minute.")
	}
	userId := uuid.New().String()

	sessionNonceBytes := make([]byte, 32)
	rand.Read(sessionNonceBytes)
	sessionNonceB64 := base64.RawURLEncoding.EncodeToString(sessionNonceBytes)

	sessionRedisKey := fmt.Sprintf("reg_session:%s", sessionNonceB64)
	sessionRedisPayload, _ := json.Marshal(redisRegistrationSession{
		HashedEmailHex:   hashedEmailHex,
		UserID:           userId,
		TrackM1RequestID: r.Context().Value(contextkeys.RequestIDKey).(string),
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

func validateM3Payload(m3 *dto.M3) error {
	if m3.User.Email == "" {
		return fmt.Errorf("m3 is missing email")
	}
	if err := utils.Base64RawUrlDecodedByteLengthCompareWith(m3.User.APAKE.RegistrationNonce, crypto.RegistrationNonceBytes); err != nil {
		return fmt.Errorf("m3 validation error: %v. check nonce failed`", err)
	}
	if err := utils.Base64RawUrlDecodedByteLengthCompareWith(m3.User.APAKE.RegistrationRecord, crypto.OpaqueRegistrationRecordBytes); err != nil {
		return fmt.Errorf("m3 validation error: %v. check record failed`", err)
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

	//Inject the M1 (previous) request_id to context for logging and fraud tracking purposes
	ctx := context.WithValue(r.Context(), contextkeys.TrackM1RequestIDKey, registrationSession.TrackM1RequestID)
	r = r.WithContext(ctx)

	m3.User.Email = strings.ToLower(m3.User.Email)

	hashedIncomingEmailHex, err := crypto.HashEmail([]byte(m3.User.Email))
	if err != nil {
		return apperrors.NewInternal(err)
	}

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

	reqIDStr, ok := r.Context().Value(contextkeys.RequestIDKey).(string)
	if !ok {
		return apperrors.NewInternal(fmt.Errorf("request ID not found in context"))
	}
	var cfRay string = ""
	if config.Cfg.Env == "production" {
		cfRay, ok = r.Context().Value(contextkeys.CFRayKey).(string)
		if !ok {
			return apperrors.NewInternal(fmt.Errorf("cf ray not found in context"))
		}
	}
	emailID := uuid.NewString()

	// Handle database lookups & inserting + sending emails in the background worker
	// This also prevents timing attacks to enumerate the emails registered in our system
	task, err := worker.NewSignupProcessingTask(cfRay, reqIDStr, emailID, m3.User.Email, registrationSession.HashedEmailHex, userID, passwordFileRecord, &m3)
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
