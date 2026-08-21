package httputils

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"quartz/pkg/app-errors"
	"quartz/pkg/contextkeys"
	"quartz/pkg/logger"

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

		// NEW: Catch panics, push to the sentryhttp context hub, and return JSON!
		defer func() {
			if rec := recover(); rec != nil {
				err := fmt.Errorf("panic: %v", rec)
				logger.ErrorContext(ctx, "PANIC RECOVERED", err)

				if hub := sentry.GetHubFromContext(ctx); hub != nil {
					hub.WithScope(func(scope *sentry.Scope) {
						scope.SetTag("request_id", reqID)
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
				logger.ErrorContext(ctx, "Internal Server Error", appErr.Err)
				if hub := sentry.GetHubFromContext(ctx); hub != nil {
					hub.WithScope(func(scope *sentry.Scope) {
						scope.SetTag("request_id", reqID)
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
		logger.ErrorContext(ctx, "Unhandled Raw Error", err)
		if hub := sentry.GetHubFromContext(ctx); hub != nil {
			hub.WithScope(func(scope *sentry.Scope) {
				scope.SetTag("request_id", reqID)
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
