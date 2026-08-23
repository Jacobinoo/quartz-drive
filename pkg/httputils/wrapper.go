package httputils

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"quartz/pkg/app-errors"
	"quartz/pkg/contextkeys"

	"github.com/getsentry/sentry-go"
)

// APIHandler forces your handlers to return errors instead of calling w.Write themselves!
type APIHandler func(w http.ResponseWriter, r *http.Request) error

type ErrorResponse struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"requestId,omitempty"`
}

func Wrap(h APIHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		reqID, _ := ctx.Value(contextkeys.RequestIDKey).(string)

		// Catch panics, push to the sentryhttp context hub, and return JSON!
		defer func() {
			if rec := recover(); rec != nil {
				err := fmt.Errorf("panic: %v", rec)
				slog.ErrorContext(ctx, "PANIC RECOVERED", slog.Any("error", err))

				if hub := sentry.GetHubFromContext(ctx); hub != nil {
					hub.WithScope(func(scope *sentry.Scope) {
						scope.SetTag("request_id", reqID)

						if cfRay, ok := ctx.Value(contextkeys.CFRayKey).(string); ok && cfRay != "" {
							scope.SetTag("cf_ray", cfRay)
						}

						if sessionID, ok := ctx.Value(contextkeys.SessionIDKey).(string); ok && sessionID != "" {
							scope.SetTag("session_id", sessionID)
						}
						if familyID, ok := ctx.Value(contextkeys.FamilyIDKey).(string); ok && familyID != "" {
							scope.SetTag("family_id", familyID)
						}
						if userID, ok := ctx.Value(contextkeys.UserIDKey).(string); ok && userID != "" {
							scope.SetUser(sentry.User{ID: userID})
						}
						hub.Recover(rec)
					})
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(ErrorResponse{
					Code:      apperrors.CodeInternal,
					Message:   "An internal server error occurred",
					RequestID: reqID,
				})
			}
		}()

		err := h(w, r)
		if err == nil {
			return // Success! Handler wrote its own response.
		}

		var appErr *apperrors.AppError

		// 1. If it is a known AppError from our domain logic
		if errors.As(err, &appErr) {
			// Only push to Sentry and terminal logs if it's an actual 5xx Server Error
			if appErr.Status >= 500 {
				slog.ErrorContext(ctx, "Internal Server Error", slog.Any("error", appErr.Err))
				if hub := sentry.GetHubFromContext(ctx); hub != nil {
					hub.WithScope(func(scope *sentry.Scope) {
						scope.SetTag("request_id", reqID)

						if cfRay, ok := ctx.Value(contextkeys.CFRayKey).(string); ok && cfRay != "" {
							scope.SetTag("cf_ray", cfRay)
						}

						if sessionID, ok := ctx.Value(contextkeys.SessionIDKey).(string); ok && sessionID != "" {
							scope.SetTag("session_id", sessionID)
						}
						if familyID, ok := ctx.Value(contextkeys.FamilyIDKey).(string); ok && familyID != "" {
							scope.SetTag("family_id", familyID)
						}
						if userID, ok := ctx.Value(contextkeys.UserIDKey).(string); ok && userID != "" {
							scope.SetUser(sentry.User{ID: userID})
						}
						hub.CaptureException(appErr.Err) // Capture the true underlying DB/System error!
					})
				}
			}

			// Send the safe JSON to the client
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(appErr.Status)
			json.NewEncoder(w).Encode(ErrorResponse{
				Code:      appErr.Code,
				Message:   appErr.Message,
				RequestID: reqID,
			})
			return
		}

		// 2. If it's a completely unhandled, raw Go error
		slog.ErrorContext(ctx, "Unhandled Raw Error", slog.Any("error", err))
		if hub := sentry.GetHubFromContext(ctx); hub != nil {
			hub.WithScope(func(scope *sentry.Scope) {
				scope.SetTag("request_id", reqID)

				if cfRay, ok := ctx.Value(contextkeys.CFRayKey).(string); ok && cfRay != "" {
					scope.SetTag("cf_ray", cfRay)
				}

				if sessionID, ok := ctx.Value(contextkeys.SessionIDKey).(string); ok && sessionID != "" {
					scope.SetTag("session_id", sessionID)
				}
				if familyID, ok := ctx.Value(contextkeys.FamilyIDKey).(string); ok && familyID != "" {
					scope.SetTag("family_id", familyID)
				}
				if userID, ok := ctx.Value(contextkeys.UserIDKey).(string); ok && userID != "" {
					scope.SetUser(sentry.User{ID: userID})
				}
				hub.CaptureException(err)
			})
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{
			Code:      apperrors.CodeInternal,
			Message:   "An internal server error occurred",
			RequestID: reqID,
		})
	}
}
