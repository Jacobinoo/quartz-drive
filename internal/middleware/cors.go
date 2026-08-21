package middleware

import (
	"net/http"
	"quartz/config"
	"quartz/pkg/httputils"
)

func CorsMiddleware(next httputils.APIHandler) httputils.APIHandler {
	return func(w http.ResponseWriter, r *http.Request) error {
		// Allow your frontend origin
		w.Header().Set("Access-Control-Allow-Origin", config.Cfg.FrontendURL)
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PATCH, DELETE")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-CSRF-Token, DPoP")

		// Handle preflight OPTIONS request
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return nil
		}

		return next(w, r)
	}
}
