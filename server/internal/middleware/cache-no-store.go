package middleware

import (
	"net/http"
)

// Sets Cache-Control: no-store header in response
func CacheControlNoStoreMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)

	})
}
