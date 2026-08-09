package signup

import (
	"net/http"
	"quartz/internal/middleware"
	"time"
)

func RegisterRoutes(mux *http.ServeMux, h *Handler) {
	limitIP := middleware.RateLimitIP(5, time.Minute, 2)
	mux.HandleFunc("/signup", middleware.CorsMiddleware(limitIP(h.Signup)))
	mux.HandleFunc("/signup/m3", middleware.CorsMiddleware(limitIP(h.SignupM3)))
}
