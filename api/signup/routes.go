package signup

import (
	"net/http"
	"quartz/config"
	"quartz/internal/middleware"
	"quartz/pkg/httputils"
	"time"

	"github.com/redis/go-redis/v9"
)

func RegisterRoutes(mux *http.ServeMux, h *Handler, rdb *redis.Client) {
	limitIP := middleware.RateLimitIP(rdb, "signup", config.Cfg.RateLimits.SignupIPRate, time.Minute, config.Cfg.RateLimits.SignupIPBurst)
	mux.HandleFunc("/signup", httputils.Wrap(middleware.CorsMiddleware(limitIP(h.Signup))))
	mux.HandleFunc("/signup/m3", httputils.Wrap(middleware.CorsMiddleware(limitIP(h.SignupM3))))
}
