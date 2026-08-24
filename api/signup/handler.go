package signup

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
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
	"strings"

	// pb "quartz/proto"

	"github.com/redis/go-redis/v9"

	"github.com/google/uuid"
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

	credID := uuid.New()

	regReqBytes, err := base64.RawURLEncoding.DecodeString(m1.RegistrationRequest)
	if err != nil {
		return apperrors.NewBadRequest("invalid base64 in registration request", err)
	}

	regResponse, err := bindings.StartRegistration(
		h.opaqueSetup,
		regReqBytes,
		[]byte(credID.String()),
	)

	var registrationResponse dto.M2

	if err == nil {
		registrationResponse = dto.M2{
			Status:               "ok",
			RegistrationResponse: base64.RawURLEncoding.EncodeToString(regResponse),
			Nonce:                credID.String(), // Use the generated UUID as the nonce
		}
		w.WriteHeader(http.StatusOK)
	} else {
		registrationResponse = dto.M2{
			Status:               "error",
			RegistrationResponse: "",
			Nonce:                "",
		}
		w.WriteHeader(http.StatusBadRequest)
	}

	log.Printf("user wants to register, waiting for m3, registration request %s", m1.RegistrationRequest)

	err = json.NewEncoder(w).Encode(registrationResponse)
	if err != nil {
		return apperrors.NewInternal(err)
	}
	return nil
}

func (h *Handler) SignupM3(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodPost {
		return apperrors.NewMethodNotAllowed("method not allowed")
	}

	var m3 dto.M3

	if err := json.NewDecoder(r.Body).Decode(&m3); err != nil {
		return apperrors.NewBadRequest("invalid request", err)
	}

	uuidString := m3.User.APAKE.RegistrationNonce
	credID, uuidParseErr := uuid.Parse(uuidString)
	if uuidParseErr != nil {
		return apperrors.NewBadRequest("invalid user identifier in nonce", uuidParseErr)
	}

	regRecordBytes, err := base64.RawURLEncoding.DecodeString(m3.User.APAKE.RegistrationRecord)
	if err != nil {
		return apperrors.NewBadRequest("invalid base64 in registration record", err)
	}

	passwordFileRecord, err := bindings.FinishRegistration(
		regRecordBytes,
	)

	if err != nil {
		log.Printf("bindings FinishRegistration call failed: %v", err)
		return apperrors.NewInternal(err)
	} else {
		log.Println("bindings FinishRegistration call succeeded")
	}

	hashedEmail, err := crypto.HashEmail([]byte(m3.User.Email))
	if err != nil {
		return apperrors.NewInternal(err)
	}

	var user model.User
	err = h.db.First(&user, "hashed_email = ?", hashedEmail).Error

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
			UserID:                               credID,
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
			ID:                 credID,
			EncryptedEmail:     encryptedEmail,
			HashedEmail:        hashedEmail,
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
			OwnerID:                credID,
			SharePublicKey:         m3.Drive.DefaultShare.PublicKey,
			WrappedSharePrivateKey: m3.Drive.DefaultShare.WrappedPrivateKey,
			SharePrivNonce:         m3.Drive.DefaultShare.PrivKeyNonce,
		}

		storedNode := model.Node{
			ID:                nodeUUID,
			Type:              model.NodeTypeFolder,
			EncryptedMetadata: "",
			MetadataNonce:     "",
			OwnerID:           credID,
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
			AuthorID:                      credID,
		}

		storedShareMember := model.ShareMember{
			ShareID:                        shareUUID,
			UserID:                         credID,
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
