package signout

import (
	"net/http"
	"quartz/internal/middleware"
)

func RegisterRoutes(mux *http.ServeMux, h *Handler) {
	mux.HandleFunc("/signout", middleware.CorsMiddleware(h.Signout))
}
