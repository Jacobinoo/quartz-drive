package httputils

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"quartz/pkg/contextkeys"
)

func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := r.Header.Get("X-Request-Id")
		if reqID == "" {
			reqID = uuid.NewString()
		}

		ctx := context.WithValue(r.Context(), contextkeys.RequestIDKey, reqID)

		// Always return it to the client so they can quote it in bug reports
		w.Header().Set("X-Request-Id", reqID)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
