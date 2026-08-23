package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"quartz/pkg/captcha"
	"strings"

	apperrors "quartz/pkg/app-errors"
	"quartz/pkg/httputils"
)

func VerifyTurnstileMiddleware(next httputils.APIHandler) httputils.APIHandler {
	return func(w http.ResponseWriter, r *http.Request) error {
		header := strings.TrimSpace(r.Header.Get("X-Verify-Token"))

		if header == "" {
			slog.WarnContext(r.Context(), "no turnstile token in request headers")
			return apperrors.NewBadRequest("invalid token", fmt.Errorf("no turnstile token in request headers"))
		}

		ip := r.Header.Get("CF-Connecting-IP")
		if ip == "" {
			ip = r.Header.Get("X-Forwarded-For")
		}
		if ip == "" {
			ip = r.Header.Get("X-Real-IP")
		}

		success, errArr, err := captcha.VerifyTurnstileToken(header, ip)
		if err != nil {
			slog.WarnContext(r.Context(), "turnstile verification failed", "errors", strings.Join(errArr, ","), "error", err)
			return apperrors.NewBadRequest("invalid token", err)
		}
		if !success {
			slog.WarnContext(r.Context(), "turnstile verification failed", "errors", strings.Join(errArr, ","))
			return apperrors.NewBadRequest("invalid token", nil)
		}

		return next(w, r)
	}
}
