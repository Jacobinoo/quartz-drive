package devices

import (
	"net/http"
	"quartz/internal/middleware"
	"time"
)

func RegisterRoutes(mux *http.ServeMux, h *Handler) {
	limitIP := middleware.RateLimitIP(300, time.Second, 100)
	limitUser := middleware.RateLimitUser(6, time.Minute, 30)

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
