package recovery

import (
	"net/http"
	"quartz/config"
	"quartz/internal/middleware"
	"quartz/pkg/httputils"
	"time"
)

func RegisterRoutes(mux *http.ServeMux, h *Handler) {
	limitIP := middleware.RateLimitIP(h.redis, "recovery", config.Cfg.RateLimits.RecoveryIPRate, time.Minute, config.Cfg.RateLimits.RecoveryIPBurst)

	mux.HandleFunc("/recovery/start", httputils.Wrap(middleware.CorsMiddleware(limitIP(h.RecoveryStart))))
	mux.HandleFunc("/recovery/session/{id}/verify", httputils.Wrap(middleware.CorsMiddleware(limitIP(h.RecoveryVerify))))
	mux.HandleFunc("/recovery/session/{id}/opaque/m1", httputils.Wrap(middleware.CorsMiddleware(limitIP(h.RecoveryOpaqueM1))))
	mux.HandleFunc("/recovery/session/{id}/complete", httputils.Wrap(middleware.CorsMiddleware(limitIP(h.RecoveryComplete))))
}
