package signup

import (
	"net/http"
	"quartz/internal/middleware"
	"time"
	"github.com/redis/go-redis/v9"
)

func RegisterRoutes(mux *http.ServeMux, h *Handler, rdb *redis.Client) {
	limitIP := middleware.RateLimitIP(rdb, "signup", 5, time.Minute, 2)
	mux.HandleFunc("/signup", middleware.CorsMiddleware(limitIP(h.Signup)))
	mux.HandleFunc("/signup/m3", middleware.CorsMiddleware(limitIP(h.SignupM3)))
}
