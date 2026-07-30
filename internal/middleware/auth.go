package middleware

import (
	"context"
	"crypto/x509"
	"encoding/hex"
	"log"
	"net/http"
	"quartz/config"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// AuthContextKey is a custom type to prevent context key collisions
type AuthContextKey string

const (
	UserIDKey AuthContextKey = "userID"
	EmailKey  AuthContextKey = "email"
)

func AccessTokenMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 1. Extract Token from Authorization Header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "DPoP ") {
			http.Error(w, "missing access token", http.StatusUnauthorized)
			return
		}
		tokenString := strings.TrimPrefix(authHeader, "DPoP ")

		// 2. Parse and Verify the JWT Signature using your ES256 Public Key
		token, err := jwt.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
			// Ensure the token method is actually ES256
			if _, ok := t.Method.(*jwt.SigningMethodECDSA); !ok {
				return nil, jwt.ErrSignatureInvalid
			}

			// Reconstruct your Public Key from the Secret Key in config
			privateKeyHex := config.Cfg.JWT.SecretKey
			privateKeyBytes, _ := hex.DecodeString(privateKeyHex)
			privateKey, _ := x509.ParseECPrivateKey(privateKeyBytes)

			// Return the Public Key to verify the signature!
			return &privateKey.PublicKey, nil
		})

		if err != nil || !token.Valid {
			http.Error(w, "invalid or expired access token", http.StatusUnauthorized)
			return
		}

		// 3. Extract the Claims
		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			http.Error(w, "invalid token claims", http.StatusUnauthorized)
			return
		}

		// Extract User ID ('sub')
		userIDStr, _ := claims["sub"].(string)
		userID, err := uuid.Parse(userIDStr)
		if err != nil {
			http.Error(w, "invalid user ID in token", http.StatusUnauthorized)
			return
		}

		// Extract Email ('email')
		email, _ := claims["email"].(string)

		// 4. Inject Data into Context
		ctx := context.WithValue(r.Context(), UserIDKey, userID)
		ctx = context.WithValue(ctx, EmailKey, email)

		log.Printf("context %s %s", email, userID)

		// Move to the next handler
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}
