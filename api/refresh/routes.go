package refresh

import (
	"net/http"
	"quartz/config"
	"quartz/internal/middleware"
	"quartz/pkg/httputils"
	"time"

	"github.com/redis/go-redis/v9"
)

func RegisterRoutes(mux *http.ServeMux, h *Handler, rdb *redis.Client) {
	limitIP := middleware.RateLimitIP(rdb, "refresh", config.Cfg.RateLimits.RefreshIPRate, time.Minute, config.Cfg.RateLimits.RefreshIPBurst)
	mux.HandleFunc("/refresh", middleware.CorsMiddleware(limitIP(httputils.Wrap(h.Refresh))))
}
