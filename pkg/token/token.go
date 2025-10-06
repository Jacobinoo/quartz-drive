package token

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
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
	Bytes []byte
	Token string
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

func IssueAccessToken(fgpHash string) (string, time.Time) {
	var (
		privateKey *ecdsa.PrivateKey
		t          *jwt.Token
		s          string
		exp        time.Time
	)
	exp = time.Now().Add(65 * time.Second)
	//private key from env
	privateKeyHex := config.Cfg.JWT.SecretKey
	privateKeyBytes, _ := hex.DecodeString(privateKeyHex)
	privateKey, _ = x509.ParseECPrivateKey(privateKeyBytes)

	t = jwt.NewWithClaims(jwt.SigningMethodES256, jwt.MapClaims{
		"fh":  fgpHash,
		"exp": exp.Unix(),
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
	}
}
