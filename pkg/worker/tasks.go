package worker

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"quartz/config"
	"quartz/internal/dto"
	"quartz/internal/model"
	"quartz/pkg/contextkeys"
	"quartz/pkg/crypto"
	"quartz/pkg/email"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
	"github.com/resend/resend-go/v2"
	"gorm.io/gorm"
)

// TaskProcessor holds all the dependencies our background workers need
type TaskProcessor struct {
	db          *gorm.DB
	emailClient *resend.Client
	redisClient *redis.Client
}

func NewTaskProcessor(db *gorm.DB, emailClient *resend.Client, redisClient *redis.Client) *TaskProcessor {
	return &TaskProcessor{
		db:          db,
		emailClient: emailClient,
		redisClient: redisClient,
	}
}

const (
	typeSignupProcessing = "email:signup" // Looking up if user exists on signup and sending emails
)

type signupProcessingPayload struct {
	RequestID          string
	PasswordFileRecord []byte
	Email              string
	HashedEmailHex     string
	UserID             uuid.UUID
	M3                 *dto.M3
}

func NewSignupProcessingTask(requestId, email, hashedEmailHex string, userID uuid.UUID, passwordFileRecord []byte, m3 *dto.M3) (*asynq.Task, error) {
	payload, err := json.Marshal(signupProcessingPayload{RequestID: requestId, Email: email, HashedEmailHex: hashedEmailHex, UserID: userID, PasswordFileRecord: passwordFileRecord, M3: m3})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(typeSignupProcessing, payload), nil
}

func (tp *TaskProcessor) handleSignupProcessingTask(ctx context.Context, t *asynq.Task) error {
	var p signupProcessingPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("json.Unmarshal failed: %v: %w", err, asynq.SkipRetry)
	}

	ctx = context.WithValue(ctx, contextkeys.RequestIDKey, p.RequestID) // reattach corresponding request_id from http signup request to context

	taskID, ok := asynq.GetTaskID(ctx)
	if !ok {
		return fmt.Errorf("could not find task ID in context: %w", asynq.SkipRetry)
	}
	retryCount, ok := asynq.GetRetryCount(ctx)
	if !ok {
		return fmt.Errorf("could not find retry count in context: %w", asynq.SkipRetry)
	}
	maxRetry, ok := asynq.GetMaxRetry(ctx)
	if !ok {
		return fmt.Errorf("could not find max retry in context: %w", asynq.SkipRetry)
	}
	queueName, ok := asynq.GetQueueName(ctx)
	if !ok {
		return fmt.Errorf("could not find queue name in context: %w", asynq.SkipRetry)
	}

	taskLogger := slog.With(
		"task_id", taskID,
		"attempt", retryCount,
		"max_attempts", maxRetry,
		"queue_name", queueName,
	)

	taskLogger.InfoContext(ctx, "started processing sign up task")

	skipDbTransaction := false

	var user model.User
	err := tp.db.WithContext(ctx).First(&user, "hashed_email = ?", p.HashedEmailHex).Error
	if err == nil {
		taskLogger.InfoContext(ctx, "user already signed up")

		// If this is true - it means this exact background task created them on a previous retry!
		if user.ID == p.UserID {
			taskLogger.InfoContext(ctx, "User was created by a previous retry of this task. Proceeding to Verify email.")
			// Skip the DB transaction and jump down to sending the Verify email!
			skipDbTransaction = true
		} else {
			taskLogger.InfoContext(ctx, "User already existed from an older signup! Possible abuse! Sending 'Account Already Exists' email.", "possible_abuse", true, "hashed_email_hex", p.HashedEmailHex)

			taskLogger.DebugContext(ctx, "Sending 'Account Already Exists' email...")
			//if err := email.SendSignupVerification(ctx, tp.emailClient, p.M3.User.Email, magicLink); err != nil {
			//	taskLogger.WarnContext(ctx, "email delivery failed", "error", err)
			//	return err
			//}
			return nil
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("database internal server error: %w", err)
	}

	taskLogger.Debug("user by email not found, will sign up")

	encryptedEmail, err := crypto.EncryptEmail([]byte(p.M3.User.Email))
	if err != nil {
		return fmt.Errorf("failed to encrypt email: %w", asynq.SkipRetry)
	}

	if skipDbTransaction == true {
		taskLogger.InfoContext(ctx, "Skipping DB transaction, user already created")
	} else {
		p.M3.User.APAKE.RegistrationRecord = base64.RawURLEncoding.EncodeToString(p.PasswordFileRecord)
		err = tp.db.Transaction(func(tx *gorm.DB) error {
			storedUserKeyStore := model.UserKeyStore{
				UserID:                               p.UserID,
				MasterKdfSalt:                        p.M3.User.Keys.MasterKdfSalt,
				AccountEncryptionPublicKey:           p.M3.User.Keys.AccountEncryptionPublicKey,
				EncryptedAccountEncryptionPrivateKey: p.M3.User.Keys.EncryptedAccountEncryptionPrivateKey,
				AccountEncryptionKeyNonce:            p.M3.User.Keys.AccountEncryptionKeyNonce,
				AccountSigningPublicKey:              p.M3.User.Keys.AccountSigningPublicKey,
				EncryptedAccountSigningPrivateKey:    p.M3.User.Keys.EncryptedAccountSigningPrivateKey,
				AccountSigningKeyNonce:               p.M3.User.Keys.AccountSigningKeyNonce,

				RecoveryEncryptedAccountEncryptionPrivateKey: p.M3.User.Keys.RecoveryEncryptedAccountEncryptionPrivateKey,
				RecoveryAccountEncryptionKeyNonce:            p.M3.User.Keys.RecoveryAccountEncryptionKeyNonce,
				RecoveryEncryptedAccountSigningPrivateKey:    p.M3.User.Keys.RecoveryEncryptedAccountSigningPrivateKey,
				RecoveryAccountSigningKeyNonce:               p.M3.User.Keys.RecoveryAccountSigningKeyNonce,
			}

			storedUser := model.User{
				ID:                 p.UserID,
				EncryptedEmail:     encryptedEmail,
				HashedEmail:        p.HashedEmailHex,
				RegistrationRecord: p.M3.User.APAKE.RegistrationRecord,
				RegistrationNonce:  p.M3.User.APAKE.RegistrationNonce,
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
				OwnerID:                p.UserID,
				SharePublicKey:         p.M3.Drive.DefaultShare.PublicKey,
				WrappedSharePrivateKey: p.M3.Drive.DefaultShare.WrappedPrivateKey,
				SharePrivNonce:         p.M3.Drive.DefaultShare.PrivKeyNonce,
			}

			storedNode := model.Node{
				ID:                nodeUUID,
				Type:              model.NodeTypeFolder,
				EncryptedMetadata: "",
				MetadataNonce:     "",
				OwnerID:           p.UserID,
				NodePublicKey:     p.M3.Drive.RootNode.PublicKey,
				WrappedNodeKey:    p.M3.Drive.RootNode.WrappedPrivateKey,
				NodePrivNonce:     p.M3.Drive.RootNode.PrivKeyNonce,
				Signature:         p.M3.Drive.RootNode.SignedEncryptedPassphrase,
			}

			storedLink := model.Link{
				ID:                            linkUUID,
				ParentNodeID:                  nil,
				ChildNodeID:                   &nodeUUID,
				EncryptedName:                 "",
				NameNonce:                     "",
				EncryptedNodePassphrase:       p.M3.Drive.RootNode.EncryptedPassphrase,
				SignedEncryptedNodePassphrase: p.M3.Drive.RootNode.SignedEncryptedPassphrase,
				AuthorID:                      p.UserID,
			}

			storedShareMember := model.ShareMember{
				ShareID:                        shareUUID,
				UserID:                         p.UserID,
				Permissions:                    255, // Full Admin
				EncryptedSharePassphrase:       p.M3.Drive.DefaultShare.EncryptedPassphraseForOwner,
				SignedEncryptedSharePassphrase: p.M3.Drive.DefaultShare.SignedEncryptedPassphraseForOwner,
			}

			if err := tx.Create(&storedUser).Error; err != nil {
				return fmt.Errorf("email registered already: %w", err)
			}

			if err := tx.Create(&storedUserKeyStore).Error; err != nil {
				return fmt.Errorf("cannot register keys: %w", err)
			}

			if err := tx.Create(&storedNode).Error; err != nil {
				return fmt.Errorf("cannot register node: %w", err)
			}

			if err := tx.Create(&storedLink).Error; err != nil {
				return fmt.Errorf("cannot register link: %w", err)
			}

			if err := tx.Create(&storedShare).Error; err != nil {
				return fmt.Errorf("cannot register share: %w", err)
			}

			if err := tx.Create(&storedShareMember).Error; err != nil {
				return fmt.Errorf("cannot register share member: %w", err)
			}

			return nil
		})
		if err != nil {
			return fmt.Errorf("signup db transaction error %w", err)
		}
	}

	// Generate secure token
	tokenBytes := make([]byte, 64)
	_, _ = rand.Read(tokenBytes)
	tokenStr := fmt.Sprintf("%x", tokenBytes)

	magicLink := fmt.Sprintf("%s/verify-email#token=%s", config.Cfg.App.FrontendURL, tokenStr)

	// Fallback for local development
	if config.Cfg.Env == "development" {
		taskLogger.DebugContext(ctx, "Local Development Magic Link", "email", p.Email, "magic_link", magicLink)
		return nil
	}

	if err := email.SendSignupVerification(ctx, tp.emailClient, p.M3.User.Email, magicLink); err != nil {
		taskLogger.WarnContext(ctx, "Failed to send email", "error", err)
		return fmt.Errorf("email delivery failed: %w", err)
	}

	return nil
}
