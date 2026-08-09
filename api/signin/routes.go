package signin

import (
	"net/http"
	"quartz/internal/middleware"
	"time"
)

func RegisterRoutes(mux *http.ServeMux, h *Handler) {
	limitIP := middleware.RateLimitIP(5, time.Minute, 2)
	mux.HandleFunc("/signin", middleware.CorsMiddleware(limitIP(h.Login)))
	mux.HandleFunc("/signin/m3", middleware.CorsMiddleware(limitIP(h.LoginM3)))
}
