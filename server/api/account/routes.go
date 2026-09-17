package account

import (
	"net/http"
	"quartz/internal/middleware"
	apperrors "quartz/pkg/app-errors"
	"quartz/pkg/httputils"
	"time"

	"github.com/redis/go-redis/v9"
)

func RegisterRoutes(mux *http.ServeMux, h *Handler, rdb *redis.Client) {
	limitInitKeys := middleware.RateLimitUser(rdb, "initialize_keys", 1, time.Second, 2)
	limitReauth := middleware.RateLimitUser(rdb, "reauth", 2, time.Second, 4)
	limitKeys := middleware.RateLimitUser(rdb, "account_keys", 5, time.Second, 10)

	mux.HandleFunc("/account/initialize-keys", httputils.Wrap(middleware.CorsMiddleware(middleware.DpopMiddleware(middleware.AccessTokenMiddleware(
		limitInitKeys(
			middleware.LastActivityTracker(h.db, h.InitializeKeys),
		),
	)))))

	mux.HandleFunc("/account/verify-email", httputils.Wrap(middleware.CorsMiddleware(h.VerifyEmail)))

	mux.HandleFunc("/account/reauth", httputils.Wrap(middleware.CorsMiddleware(middleware.DpopMiddleware(middleware.AccessTokenMiddleware(
		limitReauth(
			middleware.LastActivityTracker(h.db, h.Reauthenticate),
		),
	)))))

	mux.HandleFunc("/account/keys", httputils.Wrap(func(w http.ResponseWriter, r *http.Request) error {
		handler := middleware.CorsMiddleware(middleware.DpopMiddleware(middleware.AccessTokenMiddleware(
			limitKeys(
				middleware.LastActivityTracker(h.db, func(w http.ResponseWriter, r *http.Request) error {
					if r.Method == http.MethodGet {
						return h.GetKeys(w, r)
					} else if r.Method == http.MethodPut {
						return h.UpdateKeys(w, r)
					}
					return apperrors.NewMethodNotAllowed("method not allowed")
				}),
			),
		)))
		return handler(w, r)
	}))
}
