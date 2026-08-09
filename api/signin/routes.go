package signin

import (
	"net/http"
	"quartz/internal/middleware"
	"time"
	"github.com/redis/go-redis/v9"
)

func RegisterRoutes(mux *http.ServeMux, h *Handler, rdb *redis.Client) {
	limitIP := middleware.RateLimitIP(rdb, "signin", 10, time.Minute, 10)
	mux.HandleFunc("/signin", middleware.CorsMiddleware(limitIP(h.Login)))
	mux.HandleFunc("/signin/m3", middleware.CorsMiddleware(limitIP(h.LoginM3)))
}
