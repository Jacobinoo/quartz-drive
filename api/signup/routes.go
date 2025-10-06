package signup

import (
	"net/http"
	"quartz/internal/middleware"
)

func RegisterRoutes(mux *http.ServeMux, h *Handler) {
	mux.HandleFunc("/signup", middleware.CorsMiddleware(h.Signup))
	mux.HandleFunc("/signup/m3", middleware.CorsMiddleware(h.SignupM3))
}
