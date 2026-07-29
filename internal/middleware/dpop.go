package middleware

import (
	"log"
	"net/http"
	"strings"

	"quartz/pkg/dpop"
	"quartz/pkg/token"
)

func DpopMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authorizationHeader := strings.TrimSpace(r.Header.Get("Authorization"))
		dpopHeader := strings.TrimSpace(r.Header.Get("DPoP"))

		if authorizationHeader == "" || dpopHeader == "" {
			http.Error(w, "no auth or dpop header", http.StatusBadRequest)
			return
		}

		accessToken, isDPoP := strings.CutPrefix(authorizationHeader, "DPoP ")
		if !isDPoP {
			http.Error(w, "is not dpop header", http.StatusBadRequest)
			return
		}

		jkt, err := token.ParseAndValidateDpopToken(accessToken)
		if err != nil {
			http.Error(w, "not valid dpop token", http.StatusUnauthorized)
			return
		}

		jkt2, err := dpop.ValidateDpopProof(dpopHeader, r)
		if err != nil {
			log.Printf("%s", err)
			http.Error(w, "not valid dpop proof", http.StatusUnauthorized)
			return
		}

		if jkt != jkt2 {
			http.Error(w, "jkt not equals jkt2", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	}
}
