package account

import (
	"net/http"
	"quartz/internal/middleware"
	"quartz/pkg/httputils"

	"github.com/redis/go-redis/v9"
)

func RegisterRoutes(mux *http.ServeMux, handler *Handler, rdb *redis.Client) {
	mux.HandleFunc("/account/forgot-password", middleware.CorsMiddleware(httputils.Wrap(handler.ForgotPassword)))
	mux.HandleFunc("/account/verify-reset-code", middleware.CorsMiddleware(httputils.Wrap(handler.VerifyResetCode)))
	mux.HandleFunc("/account/reset-password/m1", middleware.CorsMiddleware(httputils.Wrap(handler.ResetPasswordM1)))
	mux.HandleFunc("/account/reset-password/m3", middleware.CorsMiddleware(httputils.Wrap(handler.ResetPasswordM3)))
}
