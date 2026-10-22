package middleware

import "net/http"

func LimitBodySize(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Limit to 1MB. If the body exceeds this, Go automatically returns a 413 Payload Too Large
		r.Body = http.MaxBytesReader(w, r.Body, 1*1024*1024)
		next.ServeHTTP(w, r)
	})
}
