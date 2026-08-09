package devices

import (
	"net/http"
	"quartz/internal/middleware"
	"time"
	"github.com/redis/go-redis/v9"
)

func RegisterRoutes(mux *http.ServeMux, h *Handler, rdb *redis.Client) {
	limitIP := middleware.RateLimitIP(rdb, "devices", 300, time.Second, 100)
	limitUser := middleware.RateLimitUser(rdb, "devices", 6, time.Minute, 30)

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
