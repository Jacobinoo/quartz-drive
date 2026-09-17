package worker

import (
	"log/slog"
	"os"

	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
	"github.com/resend/resend-go/v2"
	"gorm.io/gorm"
)

type slogLogger struct{}

func (l *slogLogger) Debug(args ...interface{}) {
	slog.Debug("asynq", "msg", args)
}

func (l *slogLogger) Info(args ...interface{}) {
	slog.Info("asynq", "msg", args)
}

func (l *slogLogger) Warn(args ...interface{}) {
	slog.Warn("asynq", "msg", args)
}

func (l *slogLogger) Error(args ...interface{}) {
	slog.Error("asynq", "msg", args)
}

func (l *slogLogger) Fatal(args ...interface{}) {
	slog.Error("asynq fatal", "msg", args)
	os.Exit(1)
}

func InitBackgroundWorkerServer(db *gorm.DB, emailClient *resend.Client, redisClient *redis.Client) *asynq.Server {
	srv := asynq.NewServer(
		asynq.RedisClientOpt{
			Addr: redisClient.Options().Addr,
		},
		asynq.Config{
			// How many concurrent workers to use
			Concurrency: 10,
			Logger:      &slogLogger{},
		},
	)

	processor := NewTaskProcessor(db, emailClient, redisClient)

	mux := asynq.NewServeMux()
	mux.HandleFunc(typeSignupProcessing, processor.handleSignupProcessingTask)
	mux.HandleFunc(typeEmailRecovery, processor.handleEmailRecoveryTask)

	if err := srv.Start(mux); err != nil {
		slog.Error("could not start background worker server", "error", err)
		os.Exit(1)
	}

	return srv
}

func InitBackgroundWorkerClient(redisClient *redis.Client) *asynq.Client {
	client := asynq.NewClient(asynq.RedisClientOpt{
		Addr: redisClient.Options().Addr,
	})

	return client
}
