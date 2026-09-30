package demo

import (
	"net/http"
	"quartz/config"
	"quartz/internal/middleware"
	"quartz/pkg/httputils"
	"time"
)

func RegisterRoutes(mux *http.ServeMux, h *Handler) {
	limitIP := middleware.RateLimitIP(h.redis, "demo", config.Cfg.RateLimits.DemoIPRate, time.Minute, config.Cfg.RateLimits.DemoIPBurst)

	mux.HandleFunc("/demo/start", httputils.Wrap(middleware.CorsMiddleware(limitIP(h.DemoStart))))

	//mux.HandleFunc("GET /demo/lock", httputils.Wrap(h.DemoLock))

	authMW := middleware.AccessTokenMiddleware
	mux.HandleFunc("/demo/finish", httputils.Wrap(middleware.CorsMiddleware(limitIP(authMW(h.DemoFinish)))))
}
