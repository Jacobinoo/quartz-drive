package dpop

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"

	jwt "github.com/golang-jwt/jwt/v5"
)

func validateDpopProof(dpopProof string) error {
	parser := jwt.NewParser(jwt.WithValidMethods([]string{jwt.SigningMethodES256.Alg()}))

	token, err := parser.Parse(dpopProof, func(t *jwt.Token) (interface{}, error) {
		//Extract JWK from token header
		rawJwk, ok := t.Header["jwk"]
		if !ok {
			return nil, errors.New("missing jwk in dpop header")
		}

		jwkBytes, err := json.Marshal(rawJwk)
		if err != nil {
			return nil, err
		}

		pubKey, err := parseJWK(jwkBytes)
		if err != nil {
			return nil, err
		}

		thumbprint, err := computeJWKThumbprint(rawJwk)
		if err != nil {
			return nil, err
		}

		fmt.Println("JWK Thumbprint:", thumbprint)

		return pubKey, nil
	})

	if err != nil {
		return "", err
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return errors.New("invalid claims")
	}

	// Validate the 'htm' claim
	if htm, ok := claims["htm"].(string); !ok || htm != "POST" {
		return "", errors.New("invalid htm claim")
	}

	// Validate the 'htu' claim
	if htu, ok := claims["htu"].(string); !ok || htu != "https://localhost:3100/v1/signin/m3" {
		return "", errors.New("invalid htu claim")
	}

	return nil
}

func parseJWK(jwkBytes []byte) (*ecdsa.PublicKey, error) {
	var jwk struct {
		Kty string `json:"kty"`
		Crv string `json:"crv"`
		X   string `json:"x"`
		Y   string `json:"y"`
	}
	if err := json.Unmarshal(jwkBytes, &jwk); err != nil {
		return nil, err
	}
	if jwk.Kty != "EC" || jwk.Crv != "P-256" {
		return nil, errors.New("unsupported key type or curve")
	}

	xBytes, err := base64.RawURLEncoding.DecodeString(jwk.X)
	if err != nil {
		return nil, err
	}
	yBytes, err := base64.RawURLEncoding.DecodeString(jwk.Y)
	if err != nil {
		return nil, err
	}

	pubKey := &ecdsa.PublicKey{
		Curve: elliptic.P256(),
		X:     new(big.Int).SetBytes(xBytes),
		Y:     new(big.Int).SetBytes(yBytes),
	}
	return pubKey, nil
}

func computeJWKThumbprint(jwkRaw interface{}) (string, error) {
	jwkMap, ok := jwkRaw.(map[string]interface{})
	if !ok {
		return "", errors.New("invalid JWK format")
	}

	ordered := map[string]string{
		"crv": jwkMap["crv"].(string),
		"kty": jwkMap["kty"].(string),
		"x":   jwkMap["x"].(string),
		"y":   jwkMap["y"].(string),
	}

	canonicalJSON, err := json.Marshal(ordered)
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256(canonicalJSON)
	return base64.RawStdEncoding.EncodeToString(hash[:]), nil
}

func computeDpopJwkThumbprint(canonicalJwk string) string {
	// Compute the SHA-256 hash of the canonical JWK
	hash := sha256.Sum256([]byte(canonicalJwk))
	return hex.EncodeToString(hash[:])
}
