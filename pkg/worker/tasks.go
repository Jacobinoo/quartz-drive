package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"quartz/internal/dto"

	"github.com/hibiken/asynq"
)

const (
	TypeSignupProcessing = "email:signup" // Looking up if user exists on signup and sending emails
)

type SignupProcessingPayload struct {
	Email  string
	UserID string
	M3     *dto.M3
}

func NewSignupProcessingTask(email string, userID string, m3 *dto.M3) (*asynq.Task, error) {
	payload, err := json.Marshal(SignupProcessingPayload{Email: email, UserID: userID, M3: m3})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeSignupProcessing, payload), nil
}

//EXAMPLE
//func NewImageResizeTask(src string) (*asynq.Task, error) {
//	payload, err := json.Marshal(ImageResizePayload{SourceURL: src})
//	if err != nil {
//		return nil, err
//	}
//	// task options can be passed to NewTask, which can be overridden at enqueue time.
//	return asynq.NewTask(TypeImageResize, payload, asynq.MaxRetry(5), asynq.Timeout(20*time.Minute)), nil
//}

func HandleSignupProcessingTask(ctx context.Context, t *asynq.Task) error {
	var p SignupProcessingPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("json.Unmarshal failed: %v: %w", err, asynq.SkipRetry)
	}
	slog.InfoContext(ctx, "Sending Email to User: user_id=%s", p.UserID)
	// Email delivery code ...
	return nil
}
