package recovery

import (
	"crypto/ed25519"
	cryptoRand "crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"quartz/internal/bindings"
	"quartz/internal/dto"
	"quartz/internal/model"
	apperrors "quartz/pkg/app-errors"
	"quartz/pkg/captcha"
	"quartz/pkg/contextkeys"
	"quartz/pkg/crypto"
	"quartz/pkg/worker"
	"strings"
	"time"

	"github.com/go-redis/redis_rate/v10"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type Handler struct {
	db          *gorm.DB
	redis       *redis.Client
	asynqClient *asynq.Client
	opaqueSetup []byte
}

func NewHandler(db *gorm.DB, redisClient *redis.Client, asynqClient *asynq.Client, opaqueSetup []byte) *Handler {
	return &Handler{db: db, redis: redisClient, asynqClient: asynqClient, opaqueSetup: opaqueSetup}
}

func (h *Handler) RecoveryStart(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodPost {
		return apperrors.NewMethodNotAllowed("method not allowed")
	}

	var req dto.RecoveryStartRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return apperrors.NewBadRequest("invalid request body", err)
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

	sessionID := uuid.New().String()
	session := dto.RecoverySession{
		ID:        sessionID,
		Method:    req.Method,
		State:     dto.StatePending,
		ExpiresAt: time.Now().Add(15 * time.Minute),
	}

	if req.Method == "email" {
		req.Email = strings.ToLower(strings.TrimSpace(req.Email))
		emailHash, err := crypto.HashEmail([]byte(req.Email))
		if err != nil {
			return apperrors.NewInternal(err)
		}

		limiter := redis_rate.NewLimiter(h.redis)

		// 1 per 1 minute
		res1m, err := limiter.Allow(r.Context(), "recovery:1m:"+emailHash, redis_rate.Limit{Rate: 1, Burst: 1, Period: time.Minute})
		if err != nil {
			slog.Error("rate limiter error", "err", err)
		} else if res1m.Allowed == 0 {
			slog.InfoContext(r.Context(), "Rate limit (1m) exceeded for recovery.", "emailHash", emailHash)

			return json.NewEncoder(w).Encode(dto.RecoveryStartResponse{
				Capabilities: []dto.RecoveryCapability{dto.CapabilityAccount},
			})
		}

		// 3 per 24 hours
		res24h, err := limiter.Allow(r.Context(), "recovery:24h:"+emailHash, redis_rate.Limit{Rate: 3, Burst: 3, Period: 24 * time.Hour})
		if err != nil {
			slog.Error("rate limiter error", "err", err)
		} else if res24h.Allowed == 0 {
			slog.InfoContext(r.Context(), "Rate limit (24h) exceeded for recovery.", "emailHash", emailHash)

			return json.NewEncoder(w).Encode(dto.RecoveryStartResponse{
				Capabilities: []dto.RecoveryCapability{dto.CapabilityAccount},
			})
		}

		reqIDStr, _ := r.Context().Value(contextkeys.RequestIDKey).(string)
		cfRay, _ := r.Context().Value(contextkeys.CFRayKey).(string)
		emailID := uuid.NewString()

		task, err := worker.NewEmailRecoveryTask(cfRay, reqIDStr, emailID, req.Email)
		if err == nil {
			h.asynqClient.Enqueue(task, asynq.Timeout(10*time.Second), asynq.MaxRetry(3))
		}

		// Always return success to prevent email enumeration
		return json.NewEncoder(w).Encode(dto.RecoveryStartResponse{
			Capabilities: []dto.RecoveryCapability{dto.CapabilityAccount},
		})
	} else if req.Method == "phrase" {
		if req.RecoveryIDHex == "" {
			return apperrors.NewBadRequest("recovery_id_hex is required", nil)
		}

		hashedRecoveryID, err := crypto.HashRecoveryID(req.RecoveryIDHex)
		if err != nil {
			return apperrors.NewInternal(err)
		}

		// TODO: Add HashedRecoveryID to model.User and query it here
		var user model.User
		if err := h.db.Preload("KeyStore").Where("hashed_recovery_id = ?", hashedRecoveryID).First(&user).Error; err != nil {
			// Do NOT silently succeed here, because phrase recovery isn't enumerable like emails
			return apperrors.NewUnauthorized("Invalid recovery phrase", nil)
		}

		session.UserID = user.ID.String()
		session.Capabilities = []dto.RecoveryCapability{dto.CapabilityAccount, dto.CapabilityData}

		// Generate Cryptographic Challenge
		challengeBytes := make([]byte, 32)
		cryptoRand.Read(challengeBytes)
		challengeHex := hex.EncodeToString(challengeBytes)
		session.Challenge = &challengeHex // Store challenge to verify signature later

		h.redis.Set(r.Context(), "recovery:session:"+sessionID, h.toJSON(session), 15*time.Minute)

		decryptedEmail, err := crypto.DecryptEmail(user.EncryptedEmail)
		var userEmail string
		if err == nil {
			userEmail = string(decryptedEmail)
		} else {
			slog.ErrorContext(r.Context(), "failed to decrypt email for recovery", "error", err)
		}

		return json.NewEncoder(w).Encode(dto.RecoveryStartResponse{
			SessionID:    sessionID,
			Capabilities: session.Capabilities,
			Email:        userEmail,
			Challenge:    &challengeHex,
			EncryptedKeys: &dto.KeysDTO{
				MasterKdfSalt:                                user.KeyStore.MasterKdfSalt,
				AccountEncryptionPublicKey:                   user.KeyStore.AccountEncryptionPublicKey,
				EncryptedAccountEncryptionPrivateKey:         user.KeyStore.EncryptedAccountEncryptionPrivateKey,
				AccountEncryptionKeyNonce:                    user.KeyStore.AccountEncryptionKeyNonce,
				AccountSigningPublicKey:                      user.KeyStore.AccountSigningPublicKey,
				EncryptedAccountSigningPrivateKey:            user.KeyStore.EncryptedAccountSigningPrivateKey,
				AccountSigningKeyNonce:                       user.KeyStore.AccountSigningKeyNonce,
				RecoveryEncryptedAccountEncryptionPrivateKey: user.KeyStore.RecoveryEncryptedAccountEncryptionPrivateKey,
				RecoveryAccountEncryptionKeyNonce:            user.KeyStore.RecoveryAccountEncryptionKeyNonce,
				RecoveryEncryptedAccountSigningPrivateKey:    user.KeyStore.RecoveryEncryptedAccountSigningPrivateKey,
				RecoveryAccountSigningKeyNonce:               user.KeyStore.RecoveryAccountSigningKeyNonce,
			},
		})
	}

	return apperrors.NewBadRequest("invalid recovery method", nil)
}

func (h *Handler) toJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func (h *Handler) RecoveryVerify(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodPost {
		return apperrors.NewMethodNotAllowed("method not allowed")
	}

	sessionID := r.PathValue("id")
	if sessionID == "" {
		return apperrors.NewBadRequest("missing session id", nil)
	}

	var req dto.RecoveryVerifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return apperrors.NewBadRequest("invalid request body", err)
	}

	sessionKey := "recovery:session:" + sessionID
	val, err := h.redis.Get(r.Context(), sessionKey).Result()
	if err != nil {
		return apperrors.NewBadRequest("invalid or expired session", nil)
	}

	var session dto.RecoverySession
	if err := json.Unmarshal([]byte(val), &session); err != nil {
		return apperrors.NewInternal(err)
	}

	if session.State != dto.StatePending && session.State != dto.StateVerified {
		return apperrors.NewBadRequest("session is not in a valid state", nil)
	}

	if session.Method == "email" {
		if req.Token == "" {
			return apperrors.NewBadRequest("token is required", nil)
		}

		hashedToken, err := crypto.HashEmail([]byte(req.Token))
		if err != nil {
			return apperrors.NewInternal(err)
		}

		if session.HashedToken == nil || *session.HashedToken != hashedToken {
			return apperrors.NewUnauthorized("invalid token", nil)
		}
	} else if session.Method == "phrase" {
		if session.State == dto.StatePending {
			if req.ChallengeSignatureHex == "" {
				return apperrors.NewBadRequest("challenge_signature_hex is required", nil)
			}

			if session.Challenge == nil {
				return apperrors.NewInternal(errors.New("missing challenge in session"))
			}
			challengeHex := *session.Challenge

			// Look up user's public signing key
			var userKeyStore model.UserKeyStore
			if err := h.db.Where("user_id = ?", session.UserID).First(&userKeyStore).Error; err != nil {
				return apperrors.NewInternal(err)
			}

			pubKeyBytes, err := base64.RawURLEncoding.DecodeString(userKeyStore.AccountSigningPublicKey)
			if err != nil {
				pubKeyBytes, err = base64.StdEncoding.DecodeString(userKeyStore.AccountSigningPublicKey)
				if err != nil {
					pubKeyBytes, err = base64.URLEncoding.DecodeString(userKeyStore.AccountSigningPublicKey)
				}
			}

			if err != nil || len(pubKeyBytes) != ed25519.PublicKeySize {
				return apperrors.NewInternal(errors.New("invalid public key stored"))
			}

			sigBytes, err := hex.DecodeString(req.ChallengeSignatureHex)
			if err != nil || len(sigBytes) != ed25519.SignatureSize {
				return apperrors.NewBadRequest("invalid signature format", nil)
			}

			challengeBytes, _ := hex.DecodeString(challengeHex)

			if !ed25519.Verify(pubKeyBytes, challengeBytes, sigBytes) {
				return apperrors.NewUnauthorized("invalid challenge signature", nil)
			}

			// Clear the challenge so it isn't reused
			session.Challenge = nil
		}
	} else {
		return apperrors.NewBadRequest("unknown method", nil)
	}

	session.State = dto.StateVerified
	h.redis.Set(r.Context(), sessionKey, h.toJSON(session), time.Until(session.ExpiresAt))

	return json.NewEncoder(w).Encode(dto.RecoveryVerifyResponse{
		State:        session.State,
		Capabilities: session.Capabilities,
	})
}

func (h *Handler) RecoveryOpaqueM1(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodPost {
		return apperrors.NewMethodNotAllowed("method not allowed")
	}

	sessionID := r.PathValue("id")
	if sessionID == "" {
		return apperrors.NewBadRequest("missing session id", nil)
	}

	var req dto.RecoveryOpaqueM1Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return apperrors.NewBadRequest("invalid request body", err)
	}

	sessionKey := "recovery:session:" + sessionID
	val, err := h.redis.Get(r.Context(), sessionKey).Result()
	if err != nil {
		return apperrors.NewBadRequest("invalid or expired session", nil)
	}

	var session dto.RecoverySession
	if err := json.Unmarshal([]byte(val), &session); err != nil {
		return apperrors.NewInternal(err)
	}

	if session.State != dto.StateVerified {
		return apperrors.NewBadRequest("session is not verified", nil)
	}

	regReqBytes, err := base64.RawURLEncoding.DecodeString(req.RegistrationRequest)
	if err != nil {
		return apperrors.NewBadRequest("invalid base64 in registration request", err)
	}

	regResponse, err := bindings.StartRegistration(
		h.opaqueSetup,
		regReqBytes,
		[]byte(session.UserID),
	)
	if err != nil {
		return apperrors.NewBadRequest("registration failed", err)
	}

	w.WriteHeader(http.StatusOK)
	return json.NewEncoder(w).Encode(dto.M2{
		Status:               "ok",
		RegistrationResponse: base64.RawURLEncoding.EncodeToString(regResponse),
	})
}

func (h *Handler) RecoveryComplete(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodPost {
		return apperrors.NewMethodNotAllowed("method not allowed")
	}

	sessionID := r.PathValue("id")
	if sessionID == "" {
		return apperrors.NewBadRequest("missing session id", nil)
	}

	var req dto.RecoveryCompleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return apperrors.NewBadRequest("invalid request body", err)
	}

	sessionKey := "recovery:session:" + sessionID
	val, err := h.redis.Get(r.Context(), sessionKey).Result()
	if err != nil {
		return apperrors.NewBadRequest("invalid or expired session", nil)
	}

	var session dto.RecoverySession
	if err := json.Unmarshal([]byte(val), &session); err != nil {
		return apperrors.NewInternal(err)
	}

	if session.State != dto.StateVerified {
		return apperrors.NewBadRequest("session is not verified", nil)
	}

	hasAccountCap := false
	hasDataCap := false
	for _, cap := range session.Capabilities {
		if cap == dto.CapabilityAccount {
			hasAccountCap = true
		}
		if cap == dto.CapabilityData {
			hasDataCap = true
		}
	}

	if hasAccountCap && req.NewOPAQUERecord == nil {
		return apperrors.NewBadRequest("new_opaque_record is required for account recovery", nil)
	}
	if hasDataCap && req.ReEncryptedKeys == nil {
		return apperrors.NewBadRequest("re_encrypted_keys is required for full recovery", nil)
	}

	// Update DB atomically
	err = h.db.Transaction(func(tx *gorm.DB) error {
		if req.NewOPAQUERecord != nil {
			if err := tx.Model(&model.User{}).Where("id = ?", session.UserID).Update("registration_record", *req.NewOPAQUERecord).Error; err != nil {
				return err
			}
		}

		if req.ReEncryptedKeys != nil {
			updates := map[string]interface{}{
				"encrypted_account_encryption_private_key": req.ReEncryptedKeys.EncryptedAccountEncryptionPrivateKey,
				"account_encryption_key_nonce":             req.ReEncryptedKeys.AccountEncryptionKeyNonce,
				"encrypted_account_signing_private_key":    req.ReEncryptedKeys.EncryptedAccountSigningPrivateKey,
				"account_signing_key_nonce":                req.ReEncryptedKeys.AccountSigningKeyNonce,
			}
			if err := tx.Model(&model.UserKeyStore{}).Where("user_id = ?", session.UserID).Updates(updates).Error; err != nil {
				return err
			}
		}

		// Revoke ALL existing sessions for this user.
		// A password reset is a destructive auth event — all other devices
		// must be forcefully logged out so they can't use stale credentials.
		if err := tx.Where("user_id = ?", session.UserID).Delete(&model.GormRefreshToken{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", session.UserID).Delete(&model.Session{}).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return apperrors.NewInternal(err)
	}

	// Consume session
	h.redis.Del(r.Context(), sessionKey)

	w.WriteHeader(http.StatusOK)
	return json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
