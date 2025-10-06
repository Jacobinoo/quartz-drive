package signin

import (
	"net/http"
	"quartz/internal/middleware"
)

func RegisterRoutes(mux *http.ServeMux, h *Handler) {
	mux.HandleFunc("/signin", middleware.CorsMiddleware(h.Login))
	mux.HandleFunc("/signin/m3", middleware.CorsMiddleware(h.LoginM3))
}
