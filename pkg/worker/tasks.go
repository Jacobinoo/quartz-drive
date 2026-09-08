package worker

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
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
	"strings"
	"time"

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
	typeSignupProcessing = "email:signup"   // Looking up if user exists on signup and sending emails
	typeEmailRecovery    = "email:recovery" // Looking up if user exists on email recovery (reset password) and sending a reset magic link
)

type signupProcessingPayload struct {
	RequestID string
	CfRay     string
	EmailID   string

	PasswordFileRecord []byte
	Email              string
	HashedEmailHex     string
	UserID             uuid.UUID
	M3                 *dto.M3
}

type emailRecoveryPayload struct {
	RequestID string
	CfRay     string
	EmailID   string

	Email string
}

func NewSignupProcessingTask(cfRay, requestId, emailID, email, hashedEmailHex string, userID uuid.UUID, passwordFileRecord []byte, m3 *dto.M3) (*asynq.Task, error) {
	payload, err := json.Marshal(signupProcessingPayload{
		RequestID:          requestId,
		CfRay:              cfRay,
		EmailID:            emailID,
		Email:              email,
		HashedEmailHex:     hashedEmailHex,
		UserID:             userID,
		PasswordFileRecord: passwordFileRecord,
		M3:                 m3})
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
	ctx = context.WithValue(ctx, contextkeys.EmailIDKey, p.EmailID)
	ctx = context.WithValue(ctx, contextkeys.CFRayKey, p.CfRay)

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

	var user model.User
	err := tp.db.WithContext(ctx).First(&user, "hashed_email = ?", p.HashedEmailHex).Error
	if err == nil {
		taskLogger.InfoContext(ctx, "User already exists! Sending 'Account Already Exists' email.")
		if err := email.SendSignupAccountExist(ctx, tp.emailClient, p.M3.User.Email); err != nil {
			taskLogger.WarnContext(ctx, "email delivery failed", "error", err)
			return err
		}
		return nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("database internal server error: %w", err)
	}

	taskLogger.DebugContext(ctx, "user by email not found, will sign up")

	p.M3.User.Email = strings.ToLower(p.M3.User.Email)

	encryptedEmail, err := crypto.EncryptEmail([]byte(p.M3.User.Email))
	if err != nil {
		return fmt.Errorf("failed to encrypt email: %w", asynq.SkipRetry)
	}

	p.M3.User.APAKE.RegistrationRecord = base64.RawURLEncoding.EncodeToString(p.PasswordFileRecord)

	// Generate secure token
	tokenBytes := make([]byte, 64)
	_, _ = rand.Read(tokenBytes)
	tokenHex := fmt.Sprintf("%x", tokenBytes)

	hashedTokenBytes := sha256.Sum256(tokenBytes)
	hashedTokenStr := fmt.Sprintf("%x", hashedTokenBytes)

	// Prepare the Redis payload
	pendingReg := dto.RedisPendingRegistration{
		UserID:             p.UserID,
		EncryptedEmail:     encryptedEmail,
		HashedEmail:        p.HashedEmailHex,
		RegistrationRecord: p.M3.User.APAKE.RegistrationRecord,
	}

	redisBytes, err := json.Marshal(pendingReg)
	if err != nil {
		return fmt.Errorf("failed to marshal pending registration: %w", err)
	}

	encryptedRedisBytes, err := crypto.EncryptRedisPayload(redisBytes)
	if err != nil {
		return fmt.Errorf("failed to encrypt redis payload: %w", err)
	}

	const redisSignupVerificationTTL = 15 * time.Minute
	// Save to Redis with a 15-minute TTL
	if err := tp.redisClient.Set(ctx, "pending_reg:"+hashedTokenStr, encryptedRedisBytes, redisSignupVerificationTTL).Err(); err != nil {
		return fmt.Errorf("failed to save pending registration to redis: %w", err)
	}

	magicLink := fmt.Sprintf("%s/verify-email#token=%s", config.Cfg.App.FrontendURL, tokenHex)

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

func NewEmailRecoveryTask(cfRay, requestId, emailID, email string) (*asynq.Task, error) {
	payload, err := json.Marshal(emailRecoveryPayload{
		RequestID: requestId,
		CfRay:     cfRay,
		EmailID:   emailID,
		Email:     email,
	})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(typeEmailRecovery, payload), nil
}

func (tp *TaskProcessor) handleEmailRecoveryTask(ctx context.Context, t *asynq.Task) error {
	var p emailRecoveryPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("json.Unmarshal failed: %v: %w", err, asynq.SkipRetry)
	}

	ctx = context.WithValue(ctx, contextkeys.RequestIDKey, p.RequestID) // reattach corresponding request_id from http forgot-password request to context
	ctx = context.WithValue(ctx, contextkeys.EmailIDKey, p.EmailID)
	ctx = context.WithValue(ctx, contextkeys.CFRayKey, p.CfRay)

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

	taskLogger.InfoContext(ctx, "started processing email recovery task")

	p.Email = strings.ToLower(strings.TrimSpace(p.Email))

	hashedEmail, err := crypto.HashEmail([]byte(p.Email))
	if err != nil {
		return fmt.Errorf("failed to hash email: %w", asynq.SkipRetry)
	}

	var user model.User
	err = tp.db.WithContext(ctx).First(&user, "hashed_email = ?", hashedEmail).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			taskLogger.InfoContext(ctx, "user not found in database, won't send a reset link")
			return nil
		}
		return fmt.Errorf("database internal server error: %w", err)
	}

	taskLogger.DebugContext(ctx, "user by email found, will send a reset link")

	// Generate a secure password reset token
	tokenBytes := make([]byte, 64)
	_, _ = rand.Read(tokenBytes)
	tokenHex := fmt.Sprintf("%x", tokenBytes)

	hashedTokenBytes := sha256.Sum256(tokenBytes)
	hashedTokenHexStr := fmt.Sprintf("%x", hashedTokenBytes)

	// Prepare the Redis payload
	pendingReg := dto.RedisPendingEmailRecovery{
		UserID:    user.ID,
		TokenHash: hashedTokenHexStr,
	}

	redisBytes, err := json.Marshal(pendingReg)
	if err != nil {
		return fmt.Errorf("failed to marshal pending email recovery: %w", err)
	}

	encryptedRedisBytes, err := crypto.EncryptRedisPayload(redisBytes)
	if err != nil {
		return fmt.Errorf("failed to encrypt redis payload: %w", err)
	}

	const redisPasswordResetTokenTTL = 15 * time.Minute
	// Save to Redis
	if err := tp.redisClient.Set(ctx, "pending_password_reset:"+hashedTokenHexStr, encryptedRedisBytes, redisPasswordResetTokenTTL).Err(); err != nil {
		return fmt.Errorf("failed to save pending email recovery to redis: %w", err)
	}

	magicLink := fmt.Sprintf("%s/reset-password#token=%s", config.Cfg.App.FrontendURL, tokenHex)

	if config.Cfg.Env == "development" {
		p.Email = config.Cfg.Email.LocalDeliveryAddress
	}

	// Send email using Resend
	if err := email.SendPasswordReset(ctx, tp.emailClient, p.Email, magicLink); err != nil {
		if config.Cfg.Env != "development" {
			taskLogger.WarnContext(ctx, "Failed to send email", "error", err)
			return fmt.Errorf("email delivery failed: %w", err)
		}
	}

	// Fallback for local development
	if config.Cfg.Env == "development" {
		taskLogger.DebugContext(ctx, "Local Development Magic Link", "email", p.Email, "magic_link", magicLink)
		return nil
	}
	return nil
}
