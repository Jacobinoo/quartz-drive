package keys

import (
	"net/http"
	"quartz/internal/middleware"
	"time"
)

func RegisterRoutes(mux *http.ServeMux, h *Handler) {
	limitIP := middleware.RateLimitIP(300, time.Second, 100)
	limitUser := middleware.RateLimitUser(100, time.Second, 50)

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
