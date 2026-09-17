package httputils

import (
	"context"
	"log/slog"
	"net/http"

	"quartz/pkg/contextkeys"

	"github.com/google/uuid"
)

func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := uuid.NewString()
		if r.Method != http.MethodOptions {
			slog.Debug("generating request ID", "reqid", reqID)
		}
		ctx := context.WithValue(r.Context(), contextkeys.RequestIDKey, reqID)

		if cfRay := r.Header.Get("CF-Ray"); cfRay != "" {
			ctx = context.WithValue(ctx, contextkeys.CFRayKey, cfRay)
		}

		w.Header().Set("X-Request-Id", reqID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
