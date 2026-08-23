package logger

import (
	"context"
	"log/slog"
	"os"
	"quartz/pkg/contextkeys"
)

var Log *slog.Logger

// ContextHandler is a custom slog.Handler that automatically extracts context values
// and appends them to every log record.
type ContextHandler struct {
	slog.Handler
}

// Handle intercepts the log record, extracts relevant context keys, and passes it to the underlying handler.
func (h *ContextHandler) Handle(ctx context.Context, r slog.Record) error {
	// Extract cf-ray
	if cfRay, ok := ctx.Value(contextkeys.CFRayKey).(string); ok && cfRay != "" {
		r.AddAttrs(slog.String("cf_ray", cfRay))
	}

	// Extract standard IDs
	if reqID, ok := ctx.Value(contextkeys.RequestIDKey).(string); ok && reqID != "" {
		r.AddAttrs(slog.String("request_id", reqID))
	}
	if sessionID, ok := ctx.Value(contextkeys.SessionIDKey).(string); ok && sessionID != "" {
		r.AddAttrs(slog.String("session_id", sessionID))
	}
	if familyID, ok := ctx.Value(contextkeys.FamilyIDKey).(string); ok && familyID != "" {
		r.AddAttrs(slog.String("family_id", familyID))
	}

	// Extract UserID if the request is authenticated
	if userID, ok := ctx.Value(contextkeys.UserIDKey).(string); ok && userID != "" {
		r.AddAttrs(slog.String("user_id", userID))
	}

	return h.Handler.Handle(ctx, r)
}

func InitLogger(env string) {
	var baseHandler slog.Handler

	opts := slog.HandlerOptions{
		Level: slog.LevelInfo,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// Rename "level" to "severity" for GCP
			if a.Key == slog.LevelKey {
				a.Key = "severity"
			}
			// Rename "msg" to "message" for GCP
			if a.Key == slog.MessageKey {
				a.Key = "message"
			}
			return a
		},
	}

	if env == "development" {
		// Pretty text format for local development
		opts.Level = slog.LevelDebug
		baseHandler = slog.NewTextHandler(os.Stdout, &opts)
	} else {
		// Pure JSON format for production (perfect for DataDog/Grafana/Prometheus)
		baseHandler = slog.NewJSONHandler(os.Stdout, &opts)
	}

	// Wrap the base handler with our custom ContextHandler
	Log = slog.New(&ContextHandler{baseHandler})
	slog.SetDefault(Log)
}
