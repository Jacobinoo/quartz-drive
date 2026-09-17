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

func (h *ContextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &ContextHandler{h.Handler.WithAttrs(attrs)} // Return a new ContextHandler wrapping the new base handler
}
func (h *ContextHandler) WithGroup(name string) slog.Handler {
	return &ContextHandler{h.Handler.WithGroup(name)}
}

// Handle intercepts the log record, extracts relevant context keys, and passes it to the underlying handler.
func (h *ContextHandler) Handle(ctx context.Context, r slog.Record) error {
	// Extract cf-ray
	if cfRay, ok := ctx.Value(contextkeys.CFRayKey).(string); ok && cfRay != "" {
		r.AddAttrs(slog.String("cf_ray", cfRay))
	}

	if email, ok := ctx.Value(contextkeys.EmailIDKey).(string); ok && email != "" {
		r.AddAttrs(slog.String("email_id", email))
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

	if trackM1RequestID, ok := ctx.Value(contextkeys.TrackM1RequestIDKey).(string); ok && trackM1RequestID != "" {
		r.AddAttrs(slog.String("track_m1_request_id", trackM1RequestID))
	}

	return h.Handler.Handle(ctx, r)
}

func InitLogger(env string) {
	var baseHandler slog.Handler

	if env == "development" {
		// Pretty text format for local development
		baseHandler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})
	} else {
		// Pure JSON format for production (perfect for DataDog/Grafana/Prometheus)
		baseHandler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})
	}

	hostname, _ := os.Hostname()

	// Wrap the base handler with our custom ContextHandler, and permanently attach the container ID
	Log = slog.New(&ContextHandler{baseHandler}).With("container_id", hostname)
	slog.SetDefault(Log)
}
