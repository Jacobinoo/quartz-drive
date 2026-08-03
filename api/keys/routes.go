package keys

import (
	"net/http"
	"quartz/internal/middleware"
)

func RegisterRoutes(mux *http.ServeMux, h *Handler) {
	mux.HandleFunc("/keys", middleware.CorsMiddleware(middleware.DpopMiddleware(middleware.AccessTokenMiddleware(h.GetUserKeys))))
}
