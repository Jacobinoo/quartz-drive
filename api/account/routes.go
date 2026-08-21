package account

import (
	"net/http"
	"quartz/internal/middleware"
	"quartz/pkg/httputils"
)

func RegisterRoutes(mux *http.ServeMux, handler *Handler) {
	mux.HandleFunc("/account/forgot-password", httputils.Wrap(middleware.CorsMiddleware(handler.ForgotPassword)))
	mux.HandleFunc("/account/verify-reset-code", httputils.Wrap(middleware.CorsMiddleware(handler.VerifyResetCode)))
	mux.HandleFunc("/account/reset-password/m1", httputils.Wrap(middleware.CorsMiddleware(handler.ResetPasswordM1)))
	mux.HandleFunc("/account/reset-password/m3", httputils.Wrap(middleware.CorsMiddleware(handler.ResetPasswordM3)))
}
