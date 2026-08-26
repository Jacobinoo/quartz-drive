package worker

import (
	"log/slog"
	"os"

	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
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

func InitBackgroundWorkerServer(redisClient *redis.Client) *asynq.Server {
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

	// mux maps a type to a handler
	mux := asynq.NewServeMux()
	mux.HandleFunc(TypeSignupProcessing, HandleSignupProcessingTask)
	//mux.Handle(tasks.TypeImageResize, tasks.NewImageProcessor())
	// ...register other handlers...

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
