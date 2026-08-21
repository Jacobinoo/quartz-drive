package keys

import (
	"net/http"
	"quartz/config"
	"quartz/internal/middleware"
	"quartz/pkg/httputils"
	"time"

	"github.com/redis/go-redis/v9"
)

func RegisterRoutes(mux *http.ServeMux, h *Handler, rdb *redis.Client) {
	limitIP := middleware.RateLimitIP(rdb, "keys", config.Cfg.RateLimits.KeysIPRate, time.Second, config.Cfg.RateLimits.KeysIPBurst)
	limitUser := middleware.RateLimitUser(rdb, "keys", config.Cfg.RateLimits.KeysUserRate, time.Second, config.Cfg.RateLimits.KeysUserBurst)

	wrap := func(handler httputils.APIHandler) http.HandlerFunc {
		return httputils.Wrap(
			middleware.CorsMiddleware(
				limitIP(
					middleware.DpopMiddleware(
						middleware.AccessTokenMiddleware(
							limitUser(
								middleware.LastActivityTracker(h.db, handler),
							),
						),
					),
				),
			),
		)
	}

	mux.HandleFunc("/keys", wrap(h.GetUserKeys))
}
