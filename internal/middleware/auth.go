package middleware

import (
	"context"
	"crypto/x509"
	"encoding/hex"
	"log"
	"net/http"
	"quartz/config"
	"strings"

	apperrors "quartz/pkg/app-errors"
	"quartz/pkg/contextkeys"
	"quartz/pkg/httputils"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func AccessTokenMiddleware(next httputils.APIHandler) httputils.APIHandler {
	return func(w http.ResponseWriter, r *http.Request) error {
		// 1. Extract Token from Authorization Header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "DPoP ") {
			return apperrors.NewUnauthorized("missing access token", nil)
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
			return apperrors.NewUnauthorized("invalid or expired access token", err)
		}

		// 3. Extract the Claims
		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			return apperrors.NewUnauthorized("invalid token claims", nil)
		}

		// Extract User ID ('sub')
		userIDStr, _ := claims["sub"].(string)
		userID, err := uuid.Parse(userIDStr)
		if err != nil {
			return apperrors.NewUnauthorized("invalid user ID in token", err)
		}

		// Extract Email ('email')
		email, _ := claims["email"].(string)

		// Extract Session ID ('sessionId')
		sessionIdStr, _ := claims["sessionId"].(string)
		sessionId, err := uuid.Parse(sessionIdStr)
		if err != nil {
			return apperrors.NewUnauthorized("invalid session ID in token", err)
		}

		// Extract Family ID ('familyId')
		familyIdStr, _ := claims["familyId"].(string)
		familyId, err := uuid.Parse(familyIdStr)
		if err != nil {
			return apperrors.NewUnauthorized("invalid family ID in token", err)
		}

		// 4. Inject Data into Context
		ctx := context.WithValue(r.Context(), contextkeys.UserIDKey, userID)
		ctx = context.WithValue(ctx, contextkeys.EmailKey, email)
		ctx = context.WithValue(ctx, contextkeys.SessionIDKey, sessionId)
		ctx = context.WithValue(ctx, contextkeys.FamilyIDKey, familyId)

		log.Printf("context %s %s", email, userID)

		// Move to the next handler
		return next(w, r.WithContext(ctx))
	}
}
