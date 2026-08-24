package signin

import (
	"net/http"
	"quartz/config"
	"quartz/internal/middleware"
	"quartz/pkg/httputils"
	"time"

	"github.com/redis/go-redis/v9"
)

func RegisterRoutes(mux *http.ServeMux, h *Handler, rdb *redis.Client) {
	limitIP := middleware.RateLimitIP(rdb, "signin", config.Cfg.RateLimits.SigninIPRate, time.Minute, config.Cfg.RateLimits.SigninIPBurst)
	mux.HandleFunc("/signin", httputils.Wrap(middleware.CorsMiddleware(limitIP(h.Login))))
	mux.HandleFunc("/signin/m3", httputils.Wrap(middleware.CorsMiddleware(limitIP(h.LoginM3))))
}
