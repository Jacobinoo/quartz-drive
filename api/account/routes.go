package account

import (
	"net/http"

	"github.com/redis/go-redis/v9"
)

func RegisterRoutes(mux *http.ServeMux, handler *Handler, redisClient *redis.Client) {
	mux.HandleFunc("/account/forgot-password", handler.ForgotPassword)
	mux.HandleFunc("/account/verify-reset-code", handler.VerifyResetCode)
	mux.HandleFunc("/account/reset-password/m1", handler.ResetPasswordM1)
	mux.HandleFunc("/account/reset-password/m3", handler.ResetPasswordM3)
}
