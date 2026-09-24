package account

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"quartz/config"
	"quartz/internal/bindings"
	"quartz/internal/dto"
	"quartz/internal/model"
	"quartz/internal/utils"
	apperrors "quartz/pkg/app-errors"
	"quartz/pkg/captcha"
	"quartz/pkg/contextkeys"
	"quartz/pkg/crypto"
	"quartz/pkg/discord"
	"quartz/pkg/worker"
	"strings"
	"time"

	"crypto/sha256"

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

	success, errArr, err := captcha.VerifyTurnstileTokenInRequest(r)
	if err != nil {
		slog.WarnContext(r.Context(), "turnstile verification failed", "errors", strings.Join(errArr, ","), "error", err)
		return apperrors.NewBadRequest("invalid token", err)
	}
	if !success {
		slog.WarnContext(r.Context(), "turnstile verification failed", "errors", strings.Join(errArr, ","))
		return apperrors.NewBadRequest("invalid token", nil)
	}

	req.Email = strings.ToLower(req.Email)

	emailHash, err := crypto.HashEmail([]byte(req.Email))
	if err != nil {
		return apperrors.NewInternal(err)
	}

	limiter := redis_rate.NewLimiter(h.redis)

	// 1 per 1 minute
	res1m, err := limiter.Allow(r.Context(), "password_reset:1m:"+emailHash, redis_rate.Limit{Rate: 1, Burst: 1, Period: time.Minute})
	if err != nil {
		//if redis is down, we still allow the request, just without rate limit protection
		slog.Error("rate limiter error", "err", err)
	} else if res1m.Allowed == 0 {
		slog.Debug("Rate limit (1m) exceeded for forgot password.")
		return apperrors.NewRateLimited("You are being ratelimited. Try again in a minute.")
	}

	// 3 per 24 hours
	res24h, err := limiter.Allow(r.Context(), "password_reset:24h:"+emailHash, redis_rate.Limit{Rate: 3, Burst: 3, Period: 24 * time.Hour})
	if err != nil {
		slog.Error("rate limiter error", "err", err)
	} else if res24h.Allowed == 0 {
		slog.Debug("Rate limit (1m) exceeded for forgot password.")
		return apperrors.NewRateLimited("You are being ratelimited. Too many password reset attempts for this email.")
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

	task, err := worker.NewEmailRecoveryTask(cfRay, reqIDStr, emailID, req.Email)
	if err != nil {
		return apperrors.NewInternal(err)
	}

	taskTimeLimit := 10 * time.Second
	enqueuedTaskInfo, err := h.asynqClient.Enqueue(task, asynq.Timeout(taskTimeLimit), asynq.MaxRetry(3))
	if err != nil {
		return apperrors.NewInternal(err)
	}

	slog.InfoContext(r.Context(), "enqueued an email recovery task", "task_id", enqueuedTaskInfo.ID, "timeout", taskTimeLimit)

	w.WriteHeader(http.StatusAccepted)
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

	decryptedEmail, err := crypto.DecryptEmail(token.User.EncryptedEmail)
	if err != nil {
		return apperrors.NewInternal(err)
	}

	res := VerifyCodeResponse{
		Status: "ok",
		Email:  string(decryptedEmail),
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

	tokenHashBytes := sha256.Sum256([]byte(m1.Token))
	tokenHash := fmt.Sprintf("%x", tokenHashBytes)

	var token model.PasswordResetToken
	if err := h.db.Preload("User").Where("token_hash = ? AND expires_at > ?", tokenHash, time.Now()).First(&token).Error; err != nil {
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
		[]byte(token.User.ID.String()),
	)
	if err != nil {
		return apperrors.NewBadRequest("registration failed", err)
	}

	// Store the new nonce temporarily in redis
	newNonce := uuid.New().String()
	encryptedNonceData, err := crypto.EncryptRedisPayload([]byte(token.User.ID.String()))
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
	Token string      `json:"token"`
	User  dto.UserDTO `json:"user"`
}

//func (h *Handler) ResetPasswordM3(w http.ResponseWriter, r *http.Request) error {
//	if r.Method != http.MethodPost {
//		return apperrors.NewMethodNotAllowed("Provided method is not allowed")
//	}
//
//	var m3 ResetM3
//	if err := json.NewDecoder(r.Body).Decode(&m3); err != nil {
//		return apperrors.NewBadRequest("Provided request body is invalid", err)
//	}
//
//	tokenHashBytes := sha256.Sum256([]byte(m3.Token))
//	tokenHash := fmt.Sprintf("%x", tokenHashBytes)
//
//	var token model.PasswordResetToken
//	if err := h.db.Preload("User").Where("token_hash = ? AND expires_at > ?", tokenHash, time.Now()).First(&token).Error; err != nil {
//		return apperrors.NewBadRequest("invalid or expired token", err)
//	}
//
//	// Verify the nonce
//	storedEncryptedUserID, err := h.redis.Get(context.Background(), "reset:nonce:"+m3.User.APAKE.RegistrationNonce).Result()
//	if err != nil {
//		return apperrors.NewBadRequest("invalid registration nonce", err)
//	}
//
//	storedUserIDBytes, err := crypto.DecryptRedisPayload(storedEncryptedUserID)
//	if err != nil || string(storedUserIDBytes) != token.User.ID.String() {
//		return apperrors.NewBadRequest("invalid registration nonce", err)
//	}
//
//	regRecordBytes, err := base64.RawURLEncoding.DecodeString(m3.User.APAKE.RegistrationRecord)
//	if err != nil {
//		return apperrors.NewBadRequest("invalid base64 in registration record", err)
//	}
//
//	passwordFileRecord, err := bindings.FinishRegistration(
//		regRecordBytes,
//	)
//
//	if err != nil {
//		return apperrors.NewInternal(err)
//	}
//
//	err = h.db.Transaction(func(tx *gorm.DB) error {
//		// Update user registration record
//		if err := tx.Model(&token.User).Updates(map[string]interface{}{
//			"registration_record": base64.RawURLEncoding.EncodeToString(passwordFileRecord),
//			"registration_nonce":  m3.User.APAKE.RegistrationNonce,
//		}).Error; err != nil {
//			return err
//		}
//
//		// Update keystore with newly encrypted standard keys
//		// Recovery keys remain the same (unless frontend re-encrypts them too)
//		if err := tx.Model(&model.UserKeyStore{}).Where("user_id = ?", token.User.ID).Updates(map[string]interface{}{
//			"master_kdf_salt":                          m3.User.Keys.MasterKdfSalt,
//			"encrypted_account_encryption_private_key": m3.User.Keys.EncryptedAccountEncryptionPrivateKey,
//			"account_encryption_key_nonce":             m3.User.Keys.AccountEncryptionKeyNonce,
//			"encrypted_account_signing_private_key":    m3.User.Keys.EncryptedAccountSigningPrivateKey,
//			"account_signing_key_nonce":                m3.User.Keys.AccountSigningKeyNonce,
//
//			"recovery_encrypted_account_encryption_private_key": m3.User.Keys.RecoveryEncryptedAccountEncryptionPrivateKey,
//			"recovery_account_encryption_key_nonce":             m3.User.Keys.RecoveryAccountEncryptionKeyNonce,
//			"recovery_encrypted_account_signing_private_key":    m3.User.Keys.RecoveryEncryptedAccountSigningPrivateKey,
//			"recovery_account_signing_key_nonce":                m3.User.Keys.RecoveryAccountSigningKeyNonce,
//		}).Error; err != nil {
//			return err
//		}
//
//		// Delete the token so it can't be reused
//		if err := tx.Delete(&token).Error; err != nil {
//			return err
//		}
//
//		return nil
//	})
//
//	if err != nil {
//		return apperrors.NewInternal(err)
//	}
//
//	// Clean up redis
//	h.redis.Del(context.Background(), "reset:nonce:"+m3.User.APAKE.RegistrationNonce)
//
//	w.WriteHeader(http.StatusOK)
//	err = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
//	if err != nil {
//		return apperrors.NewInternal(err)
//	}
//	return nil
//}

func validateInitializeAccountKeysPayload(p *dto.InitializeAccountKeysRequest) error {
	checks := []struct {
		value    string
		expected int
	}{
		{p.User.Keys.MasterKdfSalt, crypto.MasterKdfSaltBytes},
		{p.User.Keys.AccountEncryptionPublicKey, crypto.EncryptionPublicKeyBytes},
		{p.User.Keys.EncryptedAccountEncryptionPrivateKey, crypto.EncryptedAccountEncryptionPrivateKeyBytes},
		{p.User.Keys.AccountEncryptionKeyNonce, crypto.SodiumNonceBytes},
		{p.User.Keys.AccountSigningPublicKey, crypto.AccountSigningPublicKeyBytes},
		{p.User.Keys.EncryptedAccountSigningPrivateKey, crypto.EncryptedAccountSigningPrivateKeyBytes},
		{p.User.Keys.AccountSigningKeyNonce, crypto.SodiumNonceBytes},
		{p.User.Keys.RecoveryEncryptedAccountEncryptionPrivateKey, crypto.RecoveryEncryptedAccountEncryptionPrivateKeyBytes},
		{p.User.Keys.RecoveryAccountEncryptionKeyNonce, crypto.SodiumNonceBytes},
		{p.User.Keys.RecoveryEncryptedAccountSigningPrivateKey, crypto.RecoveryEncryptedAccountSigningPrivateKeyBytes},
		{p.User.Keys.RecoveryAccountSigningKeyNonce, crypto.SodiumNonceBytes},

		{p.Drive.DefaultShare.PublicKey, crypto.SharePublicKeyBytes},
		{p.Drive.DefaultShare.WrappedPrivateKey, crypto.ShareWrappedPrivateKeyBytes},
		{p.Drive.DefaultShare.PrivKeyNonce, crypto.SodiumNonceBytes},
		{p.Drive.DefaultShare.EncryptedPassphraseForOwner, crypto.ShareEncryptedPassphraseForOwnerBytes},
		{p.Drive.DefaultShare.SignedEncryptedPassphraseForOwner, crypto.ShareSignedEncryptedPassphraseForOwnerBytes},
		{p.Drive.RootNode.PublicKey, crypto.NodePublicKeyBytes},
		{p.Drive.RootNode.WrappedPrivateKey, crypto.NodeWrappedPrivateKeyBytes},
		{p.Drive.RootNode.PrivKeyNonce, crypto.SodiumNonceBytes},
		{p.Drive.RootNode.EncryptedPassphrase, crypto.NodeEncryptedPassphraseBytes},
		{p.Drive.RootNode.SignedEncryptedPassphrase, crypto.NodeSignedEncryptedPassphraseBytes},
	}

	for i, check := range checks {
		if err := utils.Base64RawUrlDecodedByteLengthCompareWith(check.value, check.expected); err != nil {
			return fmt.Errorf("initialize account keys validation error: %v. check %d failed`", err, i)
		}
	}
	return nil
}

func (h *Handler) InitializeKeys(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodPost {
		return apperrors.NewMethodNotAllowed("Provided method is not allowed")
	}
	w.Header().Set("Content-Type", "application/json")

	var initRequest dto.InitializeAccountKeysRequest
	if err := json.NewDecoder(r.Body).Decode(&initRequest); err != nil {
		return apperrors.NewBadRequest("invalid request", err)
	}

	userID, ok := r.Context().Value(contextkeys.UserIDKey).(uuid.UUID)
	if !ok || userID == uuid.Nil {
		return apperrors.NewBadRequest("unauthorized", nil)
	}

	keysInitializedBool, ok := r.Context().Value(contextkeys.KeysInitializedKey).(bool)
	if !ok {
		return apperrors.NewInternal(fmt.Errorf("keys_initialized not found in context"))
	}
	if keysInitializedBool {
		return apperrors.NewForbidden("Keys can be initialized only once", nil)
	}

	err := validateInitializeAccountKeysPayload(&initRequest)
	if err != nil {
		slog.WarnContext(r.Context(), "validate initialize account keys payload failed", "error", err)
		return apperrors.NewBadRequest("invalid payload", err)
	}

	err = h.db.Transaction(func(tx *gorm.DB) error {

		var hashedRecoveryID *string = nil
		if initRequest.User.Keys.RecoveryIDHex != "" {
			if len(initRequest.User.Keys.RecoveryIDHex) != 64 {
				return fmt.Errorf("invalid recovery id length")
			}
			hash, err := crypto.HashRecoveryID(initRequest.User.Keys.RecoveryIDHex)
			if err != nil {
				return fmt.Errorf("failed to hash recovery id: %w", err)
			}
			hashedRecoveryID = &hash
		}

		// update the existing user, the encryption version is important here, because we set it to a non-negative number, depending on the config
		// note: encryption version (-1) - means keys are not initialized, we are changing it here, to a value in the config, which must be >= 0
		if err := tx.Model(&model.User{}).Where("id = ?", userID).Updates(model.User{
			EncryptionVersion: config.Cfg.CRYPTO.EncryptionVersion,
			KdfParams: dto.KdfParams{
				KdfAlg:      config.Cfg.CRYPTO.KdfAlg,
				KdfOpsLimit: config.Cfg.CRYPTO.KdfOpsLimit,
				KdfMemLimit: config.Cfg.CRYPTO.KdfMemLimit,
			},
		}).Error; err != nil {
			return fmt.Errorf("failed to update user encryption settings: %w", err)
		}

		if hashedRecoveryID != nil {
			if err := tx.Model(&model.User{}).Where("id = ?", userID).Update("hashed_recovery_id", hashedRecoveryID).Error; err != nil {
				return fmt.Errorf("failed to update hashed_recovery_id: %w", err)
			}
		}

		storedUserKeyStore := model.UserKeyStore{
			UserID:                               userID,
			MasterKdfSalt:                        initRequest.User.Keys.MasterKdfSalt,
			AccountEncryptionPublicKey:           initRequest.User.Keys.AccountEncryptionPublicKey,
			EncryptedAccountEncryptionPrivateKey: initRequest.User.Keys.EncryptedAccountEncryptionPrivateKey,
			AccountEncryptionKeyNonce:            initRequest.User.Keys.AccountEncryptionKeyNonce,
			AccountSigningPublicKey:              initRequest.User.Keys.AccountSigningPublicKey,
			EncryptedAccountSigningPrivateKey:    initRequest.User.Keys.EncryptedAccountSigningPrivateKey,
			AccountSigningKeyNonce:               initRequest.User.Keys.AccountSigningKeyNonce,

			RecoveryEncryptedAccountEncryptionPrivateKey: initRequest.User.Keys.RecoveryEncryptedAccountEncryptionPrivateKey,
			RecoveryAccountEncryptionKeyNonce:            initRequest.User.Keys.RecoveryAccountEncryptionKeyNonce,
			RecoveryEncryptedAccountSigningPrivateKey:    initRequest.User.Keys.RecoveryEncryptedAccountSigningPrivateKey,
			RecoveryAccountSigningKeyNonce:               initRequest.User.Keys.RecoveryAccountSigningKeyNonce,
		}

		shareUUID := uuid.New()
		linkUUID := uuid.New()
		nodeUUID := uuid.New()

		storedShare := model.Share{
			ID:                     shareUUID,
			TargetLinkID:           linkUUID,
			Type:                   "DEFAULT",
			OwnerID:                userID,
			SharePublicKey:         initRequest.Drive.DefaultShare.PublicKey,
			WrappedSharePrivateKey: initRequest.Drive.DefaultShare.WrappedPrivateKey,
			SharePrivNonce:         initRequest.Drive.DefaultShare.PrivKeyNonce,
		}

		volumeUUID := uuid.New()

		storedVolume := model.Volume{
			ID:           volumeUUID,
			Type:         model.VolumeTypePrivate,
			OwnerUserID:  &userID,
			RootNodeID:   nodeUUID,
			StorageQuota: 104857600, // 100MiB default
			StorageUsed:  0,
		}

		storedNode := model.Node{
			ID:                nodeUUID,
			Type:              model.NodeTypeFolder,
			EncryptedMetadata: "",
			MetadataNonce:     "",
			OwnerID:           userID,
			VolumeID:          volumeUUID,
			NodePublicKey:     initRequest.Drive.RootNode.PublicKey,
			WrappedNodeKey:    initRequest.Drive.RootNode.WrappedPrivateKey,
			NodePrivNonce:     initRequest.Drive.RootNode.PrivKeyNonce,
			Signature:         initRequest.Drive.RootNode.SignedEncryptedPassphrase,
		}

		storedLink := model.Link{
			ID:                            linkUUID,
			ParentNodeID:                  nil,
			ChildNodeID:                   &nodeUUID,
			EncryptedName:                 "",
			NameNonce:                     "",
			EncryptedNodePassphrase:       initRequest.Drive.RootNode.EncryptedPassphrase,
			SignedEncryptedNodePassphrase: initRequest.Drive.RootNode.SignedEncryptedPassphrase,
			AuthorID:                      userID,
		}

		storedShareMember := model.ShareMember{
			ShareID:                        shareUUID,
			UserID:                         userID,
			Permissions:                    255, // Full Admin
			EncryptedSharePassphrase:       initRequest.Drive.DefaultShare.EncryptedPassphraseForOwner,
			SignedEncryptedSharePassphrase: initRequest.Drive.DefaultShare.SignedEncryptedPassphraseForOwner,
		}

		if err := tx.Create(&storedUserKeyStore).Error; err != nil {
			return fmt.Errorf("cannot initialize keys: %w", err)
		}

		if err := tx.Create(&storedVolume).Error; err != nil {
			return fmt.Errorf("cannot initialize volume: %w", err)
		}

		if err := tx.Create(&storedNode).Error; err != nil {
			return fmt.Errorf("cannot initialize node: %w", err)
		}

		if err := tx.Create(&storedLink).Error; err != nil {
			return fmt.Errorf("cannot initialize link: %w", err)
		}

		if err := tx.Create(&storedShare).Error; err != nil {
			return fmt.Errorf("cannot initialize share: %w", err)
		}

		if err := tx.Create(&storedShareMember).Error; err != nil {
			return fmt.Errorf("cannot initialize share member: %w", err)
		}

		return nil
	})

	if err != nil {
		// User already has the crypto structure initialized
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			slog.WarnContext(r.Context(), "user attempted to initialize keys, but they already exist")
			return apperrors.NewConflict("keys are already initialized for this account", err)
		}

		// Otherwise, it's a critical database failure
		slog.ErrorContext(r.Context(), "failed to initialize account keys in database", "error", err)
		return apperrors.NewInternal(err)
	}

	w.WriteHeader(http.StatusCreated)
	err = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	if err != nil {
		return apperrors.NewInternal(err)
	}

	return nil
}

type VerifyEmailRequest struct {
	Token string `json:"token"`
}

type VerifyEmailResponse struct {
	Email string `json:"email"`
}

func (h *Handler) VerifyEmail(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodPost {
		return apperrors.NewMethodNotAllowed("Provided method is not allowed")
	}
	w.Header().Set("Content-Type", "application/json")

	var verifyRequest VerifyEmailRequest
	if err := json.NewDecoder(r.Body).Decode(&verifyRequest); err != nil {
		return apperrors.NewBadRequest("invalid request", err)
	}

	tokenBytes, err := hex.DecodeString(verifyRequest.Token)
	if err != nil {
		slog.InfoContext(r.Context(), "failed to decode token", "error", err)
		return apperrors.NewBadRequest("invalid token", err)
	}

	hashedTokenBytes := sha256.Sum256(tokenBytes)
	hashedTokenStr := fmt.Sprintf("%x", hashedTokenBytes)

	sessionRedisKey := fmt.Sprintf("pending_reg:%s", hashedTokenStr)
	sessionRedisCmd := h.redis.GetDel(r.Context(), sessionRedisKey)
	err = sessionRedisCmd.Err()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			slog.InfoContext(r.Context(), "failed to decode token", "error", err)
			return apperrors.NewBadRequest("invalid verification link or account already exists", nil)
		}
		return apperrors.NewInternal(err)
	}
	sessionRedisCmdVal := sessionRedisCmd.Val()

	decryptedRedisPayloadBytes, err := crypto.DecryptRedisPayload(sessionRedisCmdVal)
	if err != nil {
		return apperrors.NewInternal(err)
	}

	var registrationSession dto.RedisPendingRegistration
	err = json.Unmarshal(decryptedRedisPayloadBytes, &registrationSession)
	if err != nil {
		return apperrors.NewInternal(err)
	}

	storedUser := model.User{
		ID:                 registrationSession.UserID,
		EncryptedEmail:     registrationSession.EncryptedEmail,
		HashedEmail:        registrationSession.HashedEmail,
		RegistrationRecord: registrationSession.RegistrationRecord,
	}
	if err := h.db.Create(&storedUser).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {

			return apperrors.NewBadRequest("invalid verification link or account already exists", nil)
		}
		return apperrors.NewInternal(err)
	}

	decryptedEmail, err := crypto.DecryptEmail(registrationSession.EncryptedEmail)
	if err != nil {
		return apperrors.NewInternal(err)
	}

	decryptedEmailStr := string(decryptedEmail)
	if decryptedEmailStr == "" {
		return apperrors.NewInternal(fmt.Errorf("decrypted email string is empty"))
	}

	w.WriteHeader(http.StatusOK)

	domain := "@unknown"
	if parts := strings.Split(decryptedEmailStr, "@"); len(parts) == 2 {
		domain = "@" + parts[1]
	}
	discord.Notify(r.Context(), "✅ User Verified Email", "A new user ("+domain+") successfully verified their email address!", 3066993) // Green color

	res := VerifyEmailResponse{
		Email: decryptedEmailStr,
	}

	err = json.NewEncoder(w).Encode(res)
	if err != nil {
		return apperrors.NewInternal(err)
	}

	return nil
}

// Reauthenticate endpoint is required for deriving the OPAQUE exportKey client side more easily
// (e.g. when the keys are not initialized during onboarding, and exportKey is required to initialize them)
func (h *Handler) Reauthenticate(w http.ResponseWriter, r *http.Request) error {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		return apperrors.NewMethodNotAllowed("method not allowed")
	}

	var m1 dto.M1Reauthenticate
	if err := json.NewDecoder(r.Body).Decode(&m1); err != nil {
		return apperrors.NewBadRequest("invalid request", err)
	}

	userID, ok := r.Context().Value(contextkeys.UserIDKey).(uuid.UUID)
	if !ok {
		return apperrors.NewUnauthorized("unauthorized", nil)
	}

	loginReqBytes, err := base64.RawURLEncoding.DecodeString(m1.LoginRequest)
	if err != nil {
		return apperrors.NewBadRequest("invalid base64 in login request", err)
	}

	var userRegistrationRecord dto.UserRegistrationRecord
	err = h.db.Where("id = ?", userID).First(&userRegistrationRecord).Error

	var registrationRecordBytes []byte
	var credentialIDBytes []byte

	registrationRecordBytes, err = base64.RawURLEncoding.DecodeString(userRegistrationRecord.RegistrationRecord)
	if err != nil {
		return apperrors.NewInternal(err)
	}
	credentialIDBytes = []byte(userID.String())

	decryptedEmailBytes, err := crypto.DecryptEmail(userRegistrationRecord.EncryptedEmail)
	if err != nil {
		return apperrors.NewInternal(err)
	}

	decryptedEmailStr := string(decryptedEmailBytes)
	if decryptedEmailStr == "" {
		return apperrors.NewInternal(fmt.Errorf("decrypted email string is empty"))
	}

	startLogRes, _, err := bindings.StartLogin(
		h.opaqueSetup,
		registrationRecordBytes,
		loginReqBytes,
		credentialIDBytes,
	)

	if err != nil {
		slog.DebugContext(r.Context(), "bindings StartLogin call failed: %v", err)
		return apperrors.NewBadRequest("invalid request", err)
	}

	var m2 dto.M2Reauthenticate
	m2 = dto.M2Reauthenticate{
		LoginResponse: base64.RawURLEncoding.EncodeToString(startLogRes),
		Email:         decryptedEmailStr,
	}

	err = json.NewEncoder(w).Encode(m2)
	if err != nil {
		return apperrors.NewInternal(err)
	}
	return nil
}

func (h *Handler) GetKeys(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodGet {
		return apperrors.NewMethodNotAllowed("method not allowed")
	}

	userID, ok := r.Context().Value(contextkeys.UserIDKey).(uuid.UUID)
	if !ok {
		return apperrors.NewUnauthorized("unauthorized", nil)
	}

	var keyStore model.UserKeyStore
	if err := h.db.Where("user_id = ?", userID).First(&keyStore).Error; err != nil {
		return apperrors.NewNotFound("keys not found", err)
	}

	w.Header().Set("Content-Type", "application/json")
	err := json.NewEncoder(w).Encode(dto.KeysDTO{
		MasterKdfSalt:                                keyStore.MasterKdfSalt,
		AccountEncryptionPublicKey:                   keyStore.AccountEncryptionPublicKey,
		EncryptedAccountEncryptionPrivateKey:         keyStore.EncryptedAccountEncryptionPrivateKey,
		AccountEncryptionKeyNonce:                    keyStore.AccountEncryptionKeyNonce,
		AccountSigningPublicKey:                      keyStore.AccountSigningPublicKey,
		EncryptedAccountSigningPrivateKey:            keyStore.EncryptedAccountSigningPrivateKey,
		AccountSigningKeyNonce:                       keyStore.AccountSigningKeyNonce,
		RecoveryEncryptedAccountEncryptionPrivateKey: keyStore.RecoveryEncryptedAccountEncryptionPrivateKey,
		RecoveryAccountEncryptionKeyNonce:            keyStore.RecoveryAccountEncryptionKeyNonce,
		RecoveryEncryptedAccountSigningPrivateKey:    keyStore.RecoveryEncryptedAccountSigningPrivateKey,
		RecoveryAccountSigningKeyNonce:               keyStore.RecoveryAccountSigningKeyNonce,
	})
	if err != nil {
		return apperrors.NewInternal(err)
	}
	return nil
}

func (h *Handler) UpdateKeys(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodPut {
		return apperrors.NewMethodNotAllowed("method not allowed")
	}

	userID, ok := r.Context().Value(contextkeys.UserIDKey).(uuid.UUID)
	if !ok {
		return apperrors.NewUnauthorized("unauthorized", nil)
	}

	var req dto.UpdateUserKeysRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return apperrors.NewBadRequest("invalid request body", err)
	}

	if req.SignatureHex == "" {
		return apperrors.NewBadRequest("signatureHex is required", nil)
	}

	var keyStore model.UserKeyStore
	if err := h.db.Where("user_id = ?", userID).First(&keyStore).Error; err != nil {
		return apperrors.NewInternal(err)
	}

	// Verify the signature
	pubKeyBytes, err := base64.RawURLEncoding.DecodeString(keyStore.AccountSigningPublicKey)
	if err != nil {
		pubKeyBytes, err = base64.StdEncoding.DecodeString(keyStore.AccountSigningPublicKey)
		if err != nil {
			pubKeyBytes, err = base64.URLEncoding.DecodeString(keyStore.AccountSigningPublicKey)
		}
	}

	if err != nil || len(pubKeyBytes) != ed25519.PublicKeySize {
		return apperrors.NewInternal(errors.New("invalid public key stored"))
	}

	sigBytes, err := hex.DecodeString(req.SignatureHex)
	if err != nil || len(sigBytes) != ed25519.SignatureSize {
		return apperrors.NewBadRequest("invalid signature format", nil)
	}

	// The frontend must sign the exact concatenation of these four fields in this order
	signedPayload := req.EncryptedAccountEncryptionPrivateKey + req.AccountEncryptionKeyNonce + req.EncryptedAccountSigningPrivateKey + req.AccountSigningKeyNonce

	if !ed25519.Verify(pubKeyBytes, []byte(signedPayload), sigBytes) {
		return apperrors.NewUnauthorized("invalid keys signature", nil)
	}

	// Signature is valid! Safe to overwrite the keys
	updates := map[string]interface{}{
		"encrypted_account_encryption_private_key": req.EncryptedAccountEncryptionPrivateKey,
		"account_encryption_key_nonce":             req.AccountEncryptionKeyNonce,
		"encrypted_account_signing_private_key":    req.EncryptedAccountSigningPrivateKey,
		"account_signing_key_nonce":                req.AccountSigningKeyNonce,
	}

	if err := h.db.Model(&keyStore).Updates(updates).Error; err != nil {
		return apperrors.NewInternal(err)
	}

	w.WriteHeader(http.StatusOK)
	return json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
