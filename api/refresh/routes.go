package refresh

import (
	"net/http"
	"quartz/internal/middleware"
	"time"
)

func RegisterRoutes(mux *http.ServeMux, h *Handler) {
	limitIP := middleware.RateLimitIP(10, time.Minute, 5)
	mux.HandleFunc("/refresh", middleware.CorsMiddleware(limitIP(h.Refresh)))
}
