package account

import (
	"net/http"
	"quartz/internal/middleware"
	"quartz/pkg/httputils"
	"time"

	"github.com/redis/go-redis/v9"
)

func RegisterRoutes(mux *http.ServeMux, h *Handler, rdb *redis.Client) {
	limitPerUser := middleware.RateLimitUser(rdb, "initialize_keys", 1, time.Second, 2)

	mux.HandleFunc("/account/forgot-password", httputils.Wrap(middleware.CorsMiddleware(h.ForgotPassword)))

	mux.HandleFunc("/account/verify-reset-code", httputils.Wrap(middleware.CorsMiddleware(h.VerifyResetCode)))

	mux.HandleFunc("/account/reset-password/m1", httputils.Wrap(middleware.CorsMiddleware(h.ResetPasswordM1)))
	//mux.HandleFunc("/account/reset-password/m3", httputils.Wrap(middleware.CorsMiddleware(h.ResetPasswordM3)))

	mux.HandleFunc("/account/initialize-keys", httputils.Wrap(middleware.CorsMiddleware(middleware.DpopMiddleware(middleware.AccessTokenMiddleware(
		limitPerUser(
			middleware.LastActivityTracker(h.db, h.InitializeKeys),
		),
	)))))

	mux.HandleFunc("/account/verify-email", httputils.Wrap(middleware.CorsMiddleware(h.VerifyEmail)))
}
