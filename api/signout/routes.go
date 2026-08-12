package signout

import (
	"net/http"
	"quartz/config"
	"quartz/internal/middleware"
	"time"

	"github.com/redis/go-redis/v9"
)

func RegisterRoutes(mux *http.ServeMux, h *Handler, rdb *redis.Client) {
	limitIP := middleware.RateLimitIP(rdb, "signout", config.Cfg.RateLimits.SignoutIPRate, time.Minute, config.Cfg.RateLimits.SignoutIPBurst)
	mux.HandleFunc("/signout", middleware.CorsMiddleware(limitIP(h.Signout)))
}
