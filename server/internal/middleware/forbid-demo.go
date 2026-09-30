package middleware

import (
	"fmt"
	"net/http"
	apperrors "quartz/pkg/app-errors"
	"quartz/pkg/contextkeys"
	"quartz/pkg/httputils"
)

// Use only after the AccessTokenMiddleware, otherwise will throw an error
func ForbidDemoMiddleware(next httputils.APIHandler) httputils.APIHandler {
	return func(w http.ResponseWriter, r *http.Request) error {
		isDemo, ok := r.Context().Value(contextkeys.IsDemoKey).(bool)
		if !ok {
			return apperrors.NewInternal(fmt.Errorf("no demo claim in token"))
		}
		if isDemo {
			return apperrors.NewForbidden("request is forbidden, error occurred: %v", fmt.Errorf("demo accounts can't access this endpoint"))
		}

		return next(w, r)
	}
}
