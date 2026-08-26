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
	"quartz/config"
	"quartz/internal/bindings"
	"quartz/internal/dto"
	"quartz/internal/model"
	apperrors "quartz/pkg/app-errors"
	"quartz/pkg/captcha"
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

	task, err := worker.NewSignupProcessingTask(hashedIncomingEmail, userID.String(), &m3)
	if err != nil {
		return apperrors.NewInternal(err)
	}
	slog.DebugContext(r.Context(), "created a new signup processing task")

	taskTimeLimit := 10 * time.Second
	enqueuedTaskInfo, err := h.asynqClient.Enqueue(task, asynq.Timeout(taskTimeLimit))
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

	var user model.User
	err = h.db.First(&user, "hashed_email = ?", registrationSession.HashedEmailHex).Error
	if err == nil {
		slog.InfoContext(r.Context(), "user already signed up, preventing enum attacks by faking status ok")
		w.WriteHeader(http.StatusCreated)
		err = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		if err != nil {
			return apperrors.NewInternal(err)
		}
		return nil
	}

	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return apperrors.NewInternal(err)
	}

	slog.Debug("user by email not found, will sign up")

	encryptedEmail, err := crypto.EncryptEmail([]byte(m3.User.Email))
	if err != nil {
		return apperrors.NewInternal(err)
	}

	// Generate secure token
	tokenBytes := make([]byte, 64)
	if _, err := rand.Read(tokenBytes); err != nil {
		return apperrors.NewInternal(err)
	}
	//tokenStr := fmt.Sprintf("%x", tokenBytes)

	//magicLink := fmt.Sprintf("%s/verify-email#token=%s", config.Cfg.App.FrontendURL, tokenStr)

	//if err := email.SendSignupVerification(r.Context(), m3.User.Email, magicLink); err != nil {
	//	log.Printf("Failed to send email: %v", err)
	//
	//	// Fallback for local development if Resend isn't configured yet
	//	if config.Cfg.Env == "development" {
	//		slog.Debug("LOCAL DEV MAGIC LINK", "email", req.Email, "magic_link", magicLink)
	//		// Continue returning 200 OK so the dev can copy the link from the terminal
	//	} else {
	//		return apperrors.NewInternal(err)
	//	}
	//}

	m3.User.APAKE.RegistrationRecord = base64.RawURLEncoding.EncodeToString(passwordFileRecord)
	err = h.db.Transaction(func(tx *gorm.DB) error {
		storedUserKeyStore := model.UserKeyStore{
			UserID:                               userID,
			MasterKdfSalt:                        m3.User.Keys.MasterKdfSalt,
			AccountEncryptionPublicKey:           m3.User.Keys.AccountEncryptionPublicKey,
			EncryptedAccountEncryptionPrivateKey: m3.User.Keys.EncryptedAccountEncryptionPrivateKey,
			AccountEncryptionKeyNonce:            m3.User.Keys.AccountEncryptionKeyNonce,
			AccountSigningPublicKey:              m3.User.Keys.AccountSigningPublicKey,
			EncryptedAccountSigningPrivateKey:    m3.User.Keys.EncryptedAccountSigningPrivateKey,
			AccountSigningKeyNonce:               m3.User.Keys.AccountSigningKeyNonce,

			RecoveryEncryptedAccountEncryptionPrivateKey: m3.User.Keys.RecoveryEncryptedAccountEncryptionPrivateKey,
			RecoveryAccountEncryptionKeyNonce:            m3.User.Keys.RecoveryAccountEncryptionKeyNonce,
			RecoveryEncryptedAccountSigningPrivateKey:    m3.User.Keys.RecoveryEncryptedAccountSigningPrivateKey,
			RecoveryAccountSigningKeyNonce:               m3.User.Keys.RecoveryAccountSigningKeyNonce,
		}

		storedUser := model.User{
			ID:                 userID,
			EncryptedEmail:     encryptedEmail,
			HashedEmail:        hashedIncomingEmail,
			RegistrationRecord: m3.User.APAKE.RegistrationRecord,
			RegistrationNonce:  m3.User.APAKE.RegistrationNonce,
			KdfParams: dto.KdfParams{
				KdfAlg:      config.Cfg.CRYPTO.KdfAlg,
				KdfOpsLimit: config.Cfg.CRYPTO.KdfOpsLimit,
				KdfMemLimit: config.Cfg.CRYPTO.KdfMemLimit,
			},
			EncryptionVersion: config.Cfg.CRYPTO.EncryptionVersion,
		}

		shareUUID := uuid.New()
		linkUUID := uuid.New()
		nodeUUID := uuid.New()

		storedShare := model.Share{
			ID:                     shareUUID,
			TargetLinkID:           linkUUID,
			Type:                   "DEFAULT",
			OwnerID:                userID,
			SharePublicKey:         m3.Drive.DefaultShare.PublicKey,
			WrappedSharePrivateKey: m3.Drive.DefaultShare.WrappedPrivateKey,
			SharePrivNonce:         m3.Drive.DefaultShare.PrivKeyNonce,
		}

		storedNode := model.Node{
			ID:                nodeUUID,
			Type:              model.NodeTypeFolder,
			EncryptedMetadata: "",
			MetadataNonce:     "",
			OwnerID:           userID,
			NodePublicKey:     m3.Drive.RootNode.PublicKey,
			WrappedNodeKey:    m3.Drive.RootNode.WrappedPrivateKey,
			NodePrivNonce:     m3.Drive.RootNode.PrivKeyNonce,
			Signature:         m3.Drive.RootNode.SignedEncryptedPassphrase,
		}

		storedLink := model.Link{
			ID:                            linkUUID,
			ParentNodeID:                  nil,
			ChildNodeID:                   &nodeUUID,
			EncryptedName:                 "",
			NameNonce:                     "",
			EncryptedNodePassphrase:       m3.Drive.RootNode.EncryptedPassphrase,
			SignedEncryptedNodePassphrase: m3.Drive.RootNode.SignedEncryptedPassphrase,
			AuthorID:                      userID,
		}

		storedShareMember := model.ShareMember{
			ShareID:                        shareUUID,
			UserID:                         userID,
			Permissions:                    255, // Full Admin
			EncryptedSharePassphrase:       m3.Drive.DefaultShare.EncryptedPassphraseForOwner,
			SignedEncryptedSharePassphrase: m3.Drive.DefaultShare.SignedEncryptedPassphraseForOwner,
		}

		if err := tx.Create(&storedUser).Error; err != nil {
			// registrationSessions.Delete(m3.RegistrationNonce)
			return apperrors.NewConflict("email registered already", err)
		}

		if err := tx.Create(&storedUserKeyStore).Error; err != nil {
			return apperrors.NewConflict("cannot register keys", err)
		}

		if err := tx.Create(&storedNode).Error; err != nil {
			return apperrors.NewConflict("cannot register node", err)
		}

		if err := tx.Create(&storedLink).Error; err != nil {
			return apperrors.NewConflict("cannot register link", err)
		}

		if err := tx.Create(&storedShare).Error; err != nil {
			return apperrors.NewConflict("cannot register share", err)
		}

		if err := tx.Create(&storedShareMember).Error; err != nil {
			return apperrors.NewConflict("cannot register share member", err)
		}

		return nil
	})

	if err != nil {
		return err
	}

	w.WriteHeader(http.StatusCreated)

	err = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	if err != nil {
		return apperrors.NewInternal(err)
	}
	return nil
}
