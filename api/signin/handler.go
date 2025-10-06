package signin

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"quartz/internal/dto"
	"quartz/internal/model"
	"quartz/pkg/token"
	pb "quartz/proto"
	"time"

	"gorm.io/gorm"
)

type Handler struct {
	db   *gorm.DB
	grpc pb.QuartzInternalCryptoServiceClient
	ctx  context.Context
}

func NewHandler(db *gorm.DB, grpc pb.QuartzInternalCryptoServiceClient, ctx context.Context) *Handler {
	return &Handler{db: db, grpc: grpc, ctx: ctx}
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var m1 dto.M1Login
	if err := json.NewDecoder(r.Body).Decode(&m1); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var userRegistrationRecord dto.UserRegistrationRecord
	if err := h.db.Where("email = ?", m1.Email).First(&userRegistrationRecord).Error; err != nil {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}

	grpcStartLoginReq := &pb.StartLoginRequest{
		Email:              m1.Email,
		RegistrationRecord: userRegistrationRecord.RegistrationRecord,
		UserIdentifier:     userRegistrationRecord.CredentialID.String(),
		StartLoginRequest:  m1.LoginRequest,
	}

	startLogRes, startLogErr := h.grpc.StartLogin(h.ctx, grpcStartLoginReq)
	if startLogErr != nil {
		log.Printf("grpc StartLogin call failed: %v", startLogErr)
	} else {
		log.Println("grpc StartLogin call succeeded")
	}

	var m2 dto.M2Login

	if startLogErr == nil {
		m2 = dto.M2Login{
			Status:        "ok",
			LoginResponse: startLogRes.LoginResponse,
			Nonce:         startLogRes.Nonce,
		}
	} else {
		m2 = dto.M2Login{
			Status:        "not_ok",
			LoginResponse: "",
			Nonce:         "",
		}
	}

	err := json.NewEncoder(w).Encode(m2)
	if err != nil {
		log.Fatalln(err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
}

func (h *Handler) LoginM3(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var m3 dto.M3Login
	if err := json.NewDecoder(r.Body).Decode(&m3); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	grpcFinishLoginReq := &pb.FinishLoginRequest{
		Nonce:              m3.Nonce,
		FinishLoginRequest: m3.FinishLoginRequest,
	}

	finishLogRes, finishLogErr := h.grpc.FinishLogin(h.ctx, grpcFinishLoginReq)
	if finishLogErr != nil {
		log.Printf("grpc StartLogin call failed: %v", finishLogErr)
	} else {
		log.Println("grpc StartLogin call succeeded")
	}

	fmt.Println("Established a new trusted session key: ", finishLogRes.SessionKey)

	var trustedUserInfo dto.TrustedUserInformation

	if err := h.db.Where("email = ?", finishLogRes.Email).First(&trustedUserInfo).Error; err != nil {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}

	var refreshToken = token.IssueRefreshToken()
	newCsrfToken := token.IssueCsrfToken()

	var refreshTokenEntry = model.GormRefreshToken{
		TokenHash: refreshToken.TokenSha256Hash,
		UserID:    trustedUserInfo.ID,
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour), // default 7 days
		CsrfToken: newCsrfToken.Token,
	}

	fingerprintBytes := make([]byte, 64)
	rand.Read(fingerprintBytes)
	fingerprint := hex.EncodeToString(fingerprintBytes)

	fingerprintHashBytes := sha256.Sum256(fingerprintBytes)
	fingerprintHash := hex.EncodeToString(fingerprintHashBytes[:])

	accessToken, expTime := token.IssueAccessToken(fingerprintHash)
	maxAge := time.Until(expTime)

	http.SetCookie(w, &http.Cookie{
		Name:     "__Secure-F",
		Value:    fingerprint,
		HttpOnly: true,
		Path:     "/",
		MaxAge:   int(maxAge.Seconds()),
		Secure:   true,                  // change to true in production
		SameSite: http.SameSiteNoneMode, // change to http.SameSiteStrictMode in production
	})

	http.SetCookie(w, &http.Cookie{
		Name:     "__Secure-Auth",
		Value:    refreshToken.Token,
		HttpOnly: true,
		Path:     "/",
		Secure:   true,                  // change to true in production
		SameSite: http.SameSiteNoneMode, // change to http.SameSiteStrictMode in production
		Expires:  time.Now().Add(7 * 24 * time.Hour),
	})

	if err := h.db.Create(&refreshTokenEntry).Error; err != nil {
		log.Printf("failed to store refresh token: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	var loginTrustAttestation = dto.LoginTrustAttestationConfirmedBody{
		LoginTrustAttestationHeader: dto.LoginTrustAttestationHeader{
			Status:      "ok",
			Attestation: true,
		},
		TrustedUserInformation: trustedUserInfo,
		Token:                  accessToken,
		CsrfToken:              newCsrfToken.Token,
	}

	err := json.NewEncoder(w).Encode(loginTrustAttestation)
	if err != nil {
		log.Fatalln(err)
		// loginSessions.Delete(m3.Nonce)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
}
