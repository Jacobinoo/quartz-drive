package account

import (
	"net/http"

	"quartz/internal/middleware"
	"github.com/redis/go-redis/v9"
)

func RegisterRoutes(mux *http.ServeMux, handler *Handler, redisClient *redis.Client) {
	mux.HandleFunc("/account/forgot-password", middleware.CorsMiddleware(handler.ForgotPassword))
	mux.HandleFunc("/account/verify-reset-code", middleware.CorsMiddleware(handler.VerifyResetCode))
	mux.HandleFunc("/account/reset-password/m1", middleware.CorsMiddleware(handler.ResetPasswordM1))
	mux.HandleFunc("/account/reset-password/m3", middleware.CorsMiddleware(handler.ResetPasswordM3))
}
