package keys

import (
	"net/http"
	"quartz/internal/middleware"
	"time"
	"github.com/redis/go-redis/v9"
)

func RegisterRoutes(mux *http.ServeMux, h *Handler, rdb *redis.Client) {
	limitIP := middleware.RateLimitIP(rdb, "keys", 300, time.Second, 100)
	limitUser := middleware.RateLimitUser(rdb, "keys", 100, time.Second, 50)

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

	mux.HandleFunc("/keys", wrap(h.GetUserKeys))
}
