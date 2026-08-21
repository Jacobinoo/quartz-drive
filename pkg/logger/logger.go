package logger

import (
	"context"
	"log/slog"
	"os"
	"quartz/pkg/contextkeys"
)

var Log *slog.Logger

func InitLogger(env string) {
	var handler slog.Handler

	if env == "development" {
		// Pretty text format for local development
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})
	} else {
		// Pure JSON format for production (perfect for DataDog/Grafana/Prometheus)
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	}

	Log = slog.New(handler)
	slog.SetDefault(Log)
}

// ErrorContext securely logs an error to the terminal, automatically
// extracting the RequestID from the context to tag the log entry.
func ErrorContext(ctx context.Context, msg string, err error, args ...any) {
	reqID, _ := ctx.Value(contextkeys.RequestIDKey).(string)

	attrs := append([]any{
		slog.String("request_id", reqID),
		slog.Any("error", err),
	}, args...)

	Log.ErrorContext(ctx, msg, attrs...)
}
