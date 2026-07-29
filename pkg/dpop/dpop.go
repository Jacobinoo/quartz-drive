package dpop

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"strings"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func fullURL(r *http.Request) string {
	builder := strings.Builder{}

	if r.TLS != nil {
		builder.WriteString("https://")
	} else {
		builder.WriteString("http://")
	}

	builder.WriteString(r.Host)
	builder.WriteString(r.RequestURI)

	// if r.URL.RawQuery != "" {
	// 	builder.WriteString("?" + r.URL.RawQuery)
	// }

	// if r.URL.Fragment != "" {
	// 	builder.WriteString("#" + r.URL.Fragment)
	// }

	return builder.String()
}

func ValidateDpopProof(dpopProof string, r *http.Request) (string, error) {
	if dpopProof == "" {
		log.Printf("dpop proof is empty")
		return "", errors.New("invaid_dpop_proof")
	}

	parser := jwt.NewParser(jwt.WithValidMethods([]string{jwt.SigningMethodES256.Alg()}))

	thumbprint := ""

	_, err := parser.Parse(dpopProof, func(t *jwt.Token) (interface{}, error) {
		alg, ok := t.Header["alg"]
		if !ok || alg != "ES256" {
			return nil, errors.New("alg claim is invalid")
		}

		typ, ok := t.Header["typ"]
		if !ok || typ != "dpop+jwt" {
			return nil, errors.New("typ claim is invalid")
		}

		rawJwk, ok := t.Header["jwk"]
		if !ok || rawJwk == "" {
			return nil, errors.New("jwk is invalid")
		}

		htm, ok := t.Claims.(jwt.MapClaims)["htm"]
		if !ok || htm != r.Method {
			return nil, errors.New("htm claim is invalid")
		}

		htu, ok := t.Claims.(jwt.MapClaims)["htu"]
		if !ok || htu != fullURL(r) {
			log.Printf("htu %s VS requestUrl %s", htu, fullURL(r))
			return nil, errors.New("htu claim is invalid")
		}

		jti, ok := t.Claims.(jwt.MapClaims)["jti"].(string)
		_, err := uuid.Parse(jti)
		if !ok || err != nil {
			return nil, errors.New("jti claim is invalid")
		}

		iat, ok := t.Claims.(jwt.MapClaims)["iat"].(float64)
		if !ok {
			return nil, errors.New("iat claim is invalid")
		}
		if err != nil {
			return nil, errors.New("iat claim could not be parsed")
		}
		timeIat := time.Unix(int64(iat), 0)
		expired := timeIat.After(time.Now().Add(30 * time.Second))
		if expired {
			return nil, errors.New("dpop proof expired")
		}

		jwkBytes, err := json.Marshal(rawJwk)
		if err != nil {
			return nil, err
		}

		pubKey, err := parseJWK(jwkBytes)
		if err != nil {
			return nil, err
		}

		thumbprint, err = computeJWKThumbprint(rawJwk)
		if err != nil {
			return nil, err
		}

		return pubKey, nil
	})

	if err != nil {
		return "", err
	}

	fmt.Println("JWK Thumbprint:", thumbprint)
	return thumbprint, nil
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
	return base64.RawURLEncoding.EncodeToString(hash[:]), nil
}
