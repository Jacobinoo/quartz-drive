package middleware

import (
	"log"
	"net/http"
	"strings"

	apperrors "quartz/pkg/app-errors"
	"quartz/pkg/dpop"
	"quartz/pkg/httputils"
	"quartz/pkg/token"
)

func DpopMiddleware(next httputils.APIHandler) httputils.APIHandler {
	return func(w http.ResponseWriter, r *http.Request) error {
		authorizationHeader := strings.TrimSpace(r.Header.Get("Authorization"))
		dpopHeader := strings.TrimSpace(r.Header.Get("DPoP"))

		if authorizationHeader == "" || dpopHeader == "" {
			return apperrors.NewBadRequest("no auth or dpop header", nil)
		}

		accessToken, isDPoP := strings.CutPrefix(authorizationHeader, "DPoP ")
		if !isDPoP {
			return apperrors.NewBadRequest("is not dpop header", nil)
		}

		jkt, err := token.ParseAndValidateDpopToken(accessToken)
		if err != nil {
			return apperrors.NewUnauthorized("not valid dpop token", err)
		}

		jkt2, err := dpop.ValidateDpopProof(dpopHeader, r)
		if err != nil {
			log.Printf("%s", err)
			return apperrors.NewUnauthorized("not valid dpop proof", err)
		}

		if jkt != jkt2 {
			return apperrors.NewUnauthorized("jkt not equals jkt2", nil)
		}

		return next(w, r)
	}
}
