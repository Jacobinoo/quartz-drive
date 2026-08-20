package account

import (
	"net/http"
	"quartz/internal/middleware"
	"quartz/pkg/httputils"
	"time"

	"github.com/redis/go-redis/v9"
)

func RegisterRoutes(mux *http.ServeMux, handler *Handler, rdb *redis.Client) {
	rateLimiter := middleware.RateLimitIP(rdb, "account_forgot_verify", 3, time.Minute*30, 9)

	mux.HandleFunc("/account/forgot-password", middleware.CorsMiddleware(rateLimiter(httputils.Wrap(handler.ForgotPassword))))
	mux.HandleFunc("/account/verify-reset-code", middleware.CorsMiddleware(rateLimiter(httputils.Wrap(handler.VerifyResetCode))))

	rateLimiter2 := middleware.RateLimitIP(rdb, "account_reset_m1_m3", 3, time.Minute*15, 12)

	mux.HandleFunc("/account/reset-password/m1", middleware.CorsMiddleware(rateLimiter2(httputils.Wrap(handler.ResetPasswordM1))))
	mux.HandleFunc("/account/reset-password/m3", middleware.CorsMiddleware(rateLimiter2(httputils.Wrap(handler.ResetPasswordM3))))
}
