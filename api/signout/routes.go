package signout

import (
	"net/http"
	"quartz/internal/middleware"
	"time"
	"github.com/redis/go-redis/v9"
)

func RegisterRoutes(mux *http.ServeMux, h *Handler, rdb *redis.Client) {
	limitIP := middleware.RateLimitIP(rdb, "signout", 10, time.Minute, 5)
	mux.HandleFunc("/signout", middleware.CorsMiddleware(limitIP(h.Signout)))
}
