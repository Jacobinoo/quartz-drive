package devices

import (
	"net/http"
	"quartz/internal/middleware"
)

func RegisterRoutes(mux *http.ServeMux, h *Handler) {
	mux.HandleFunc("/devices/register", middleware.CorsMiddleware(middleware.DpopMiddleware(middleware.AccessTokenMiddleware(middleware.LastActivityTracker(h.db, h.RegisterDevice)))))
	mux.HandleFunc("/devices/list", middleware.CorsMiddleware(middleware.DpopMiddleware(middleware.AccessTokenMiddleware(middleware.LastActivityTracker(h.db, h.ListDevices)))))
	mux.HandleFunc("/devices/revoke", middleware.CorsMiddleware(middleware.DpopMiddleware(middleware.AccessTokenMiddleware(middleware.LastActivityTracker(h.db, h.RevokeDevice)))))
}
