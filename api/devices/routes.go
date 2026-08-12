package devices

import (
	"net/http"
	"quartz/config"
	"quartz/internal/middleware"
	"time"

	"github.com/redis/go-redis/v9"
)

func RegisterRoutes(mux *http.ServeMux, h *Handler, rdb *redis.Client) {
	limitIP := middleware.RateLimitIP(rdb, "devices", config.Cfg.RateLimits.DevicesIPRate, time.Second, config.Cfg.RateLimits.DevicesIPBurst)
	limitUser := middleware.RateLimitUser(rdb, "devices", config.Cfg.RateLimits.DevicesUserRate, time.Minute, config.Cfg.RateLimits.DevicesUserBurst)

	wrap := func(handler http.HandlerFunc) http.HandlerFunc {
		return middleware.CorsMiddleware(
			limitIP(
				middleware.DpopMiddleware(
					middleware.AccessTokenMiddleware(
						limitUser(
							middleware.LastActivityTracker(h.db, handler),
						),
					),
				),
			),
		)
	}

	mux.HandleFunc("/devices/register", wrap(h.RegisterDevice))
	mux.HandleFunc("/devices/list", wrap(h.ListDevices))
	mux.HandleFunc("/devices/revoke", wrap(h.RevokeDevice))
}
