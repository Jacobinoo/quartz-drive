package token

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"quartz/config"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type GeneratedRefreshToken struct {
	Bytes           []byte
	Token           string
	TokenSha256Hash string
}

type GeneratedCsrfToken struct {
	Bytes           []byte
	Token           string
	TokenSha256Hash string
}

func IssueRefreshToken() GeneratedRefreshToken {
	refreshTokenBytes := make([]byte, 64)
	rand.Read(refreshTokenBytes)

	return GeneratedRefreshToken{
		Bytes: refreshTokenBytes,
		Token: hex.EncodeToString(refreshTokenBytes),
		TokenSha256Hash: func() string {
			hash := sha256.Sum256(refreshTokenBytes)
			return hex.EncodeToString(hash[:])
		}(),
	}
}

func IssueAccessToken(withKeysInitialized bool, fgpHash string, dpopJkt string, userID string, email string, sessionId string, familyId string) (string, time.Time) {
	var (
		privateKey *ecdsa.PrivateKey
		t          *jwt.Token
		s          string
		exp        time.Time
	)
	const accessTokenLifetime = time.Minute * 5
	exp = time.Now().Add(accessTokenLifetime)
	//private key from env
	privateKeyHex := config.Cfg.JWT.SecretKey
	privateKeyBytes, _ := hex.DecodeString(privateKeyHex)
	privateKey, _ = x509.ParseECPrivateKey(privateKeyBytes)

	t = jwt.NewWithClaims(jwt.SigningMethodES256, jwt.MapClaims{
		"fh":    fgpHash,
		"sub":   userID,
		"email": email,
		"exp":   exp.Unix(),
		"cnf": map[string]string{
			"jkt": dpopJkt,
		},
		"sessionId":       sessionId,
		"familyId":        familyId,
		"keysInitialized": withKeysInitialized,
	})
	s, _ = t.SignedString(privateKey)

	return s, exp
}

func IssueCsrfToken() GeneratedCsrfToken {
	tokenBytes := make([]byte, 64)
	rand.Read(tokenBytes)

	return GeneratedCsrfToken{
		Bytes: tokenBytes,
		Token: hex.EncodeToString(tokenBytes),
		TokenSha256Hash: func() string {
			hash := sha256.Sum256(tokenBytes)
			return hex.EncodeToString(hash[:])
		}(),
	}
}

func ParseAndValidateDpopToken(tokenString string) (string, error) {
	// 1. Parse the token and provide the key for validation
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		// Ensure the signing method is exactly what we expect (ES256)
		if _, ok := token.Method.(*jwt.SigningMethodECDSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		// Retrieve your private key from the environment (just like in IssueAccessToken)
		privateKeyHex := config.Cfg.JWT.SecretKey
		privateKeyBytes, _ := hex.DecodeString(privateKeyHex)
		privateKey, err := x509.ParseECPrivateKey(privateKeyBytes)
		if err != nil {
			return nil, err
		}
		// Return the Public Key portion for signature verification
		return &privateKey.PublicKey, nil
	})
	if err != nil {
		return "", fmt.Errorf("invalid token: %v", err)
	}
	// 2. Extract the claims
	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {

		// 3. Navigate the nested JSON map to find "cnf" -> "jkt"
		if cnf, ok := claims["cnf"].(map[string]interface{}); ok {
			if jkt, ok := cnf["jkt"].(string); ok {
				return jkt, nil // Successfully extracted the DPoP thumbprint!
			}
		}
		return "", fmt.Errorf("token missing cnf.jkt claim")
	}
	return "", fmt.Errorf("invalid token claims")
}
