package devices

import (
	"net/http"
	"quartz/internal/middleware"
)

func RegisterRoutes(mux *http.ServeMux, h *Handler) {
	mux.HandleFunc("/devices/register", middleware.CorsMiddleware(middleware.DpopMiddleware(middleware.AccessTokenMiddleware(h.RegisterDevice))))
}
