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
	"quartz/pkg/dpop"
	"quartz/pkg/token"
	pb "quartz/proto"
	"time"

	"github.com/google/uuid"
	"github.com/mssola/useragent"

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

	dpopHeader := r.Header.Get("DPoP")
	thumbprint, err := dpop.ValidateDpopProof(dpopHeader, r)
	if err != nil {
		log.Printf("invalid dpop proof, err: %v", err)
		http.Error(w, "invalid_dpop_proof", http.StatusBadRequest)
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
		log.Printf("grpc FinishLogin call failed: %v", finishLogErr)
	} else {
		log.Println("grpc FinishLogin call succeeded")
	}

	fmt.Println("Established a new trusted session key: ", finishLogRes.SessionKey)

	var trustedUserInfo dto.TrustedUserInformation

	err = h.db.Model(&model.User{}).
		Select("users.*, user_key_stores.*").
		Joins("INNER JOIN user_key_stores ON user_key_stores.user_id = users.id").
		Where("users.email = ?", finishLogRes.Email).
		First(&trustedUserInfo).Error

	if err != nil {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}

	uaHeader := r.Header.Get("User-Agent")
	ua := useragent.New(uaHeader)

	fmt.Printf("Mobile: %v\n", ua.Mobile())   // => true
	fmt.Printf("Bot: %v\n", ua.Bot())         // => false
	fmt.Printf("Mozilla: %v\n", ua.Mozilla()) // => "5.0"
	fmt.Printf("Model: %v\n", ua.Model())     // => "Nexus One"

	fmt.Printf("Platform: %v\n", ua.Platform()) // => "Linux"
	fmt.Printf("OS: %v\n", ua.OS())             // => "Android 2.3.7"

	name, version := ua.Engine()
	fmt.Printf("Engine: %v\n", name)     // => "AppleWebKit"
	fmt.Printf("Version: %v\n", version) // => "533.1"

	name, version = ua.Browser()
	fmt.Printf("Browser: %v\n", name)    // => "Android"
	fmt.Printf("Version: %v\n", version) // => "4.0"

	displayedDeviceName := ua.OS() + " - " + ua.Model() + " - " + name

	newSessionEntry := model.Session{
		UserID:       trustedUserInfo.ID,
		UserAgent:    uaHeader,
		DeviceName:   displayedDeviceName,
		LastActiveAt: time.Now(),
	}

	familyID := uuid.New()
	refreshToken := token.IssueRefreshToken()
	newCsrfToken := token.IssueCsrfToken()

	refreshTokenEntry := model.GormRefreshToken{
		UserID:        trustedUserInfo.ID,
		TokenHash:     refreshToken.TokenSha256Hash,
		FamilyID:      familyID,
		IsRevoked:     false,
		ExpiresAt:     time.Now().Add(7 * 24 * time.Hour), // default 7 days
		CsrfTokenHash: newCsrfToken.TokenSha256Hash,
		Session:       newSessionEntry,
		DpopJKT:       thumbprint,
	}

	fingerprintBytes := make([]byte, 64)
	_, _ = rand.Read(fingerprintBytes)
	fingerprint := hex.EncodeToString(fingerprintBytes)

	fingerprintHashBytes := sha256.Sum256(fingerprintBytes)
	fingerprintHash := hex.EncodeToString(fingerprintHashBytes[:])

	accessToken, expTime := token.IssueAccessToken(fingerprintHash, thumbprint, trustedUserInfo.ID.String(), trustedUserInfo.Email)
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
		log.Printf("failed to store login data: %v", err)
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

	err = json.NewEncoder(w).Encode(loginTrustAttestation)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		// loginSessions.Delete(m3.Nonce)
		return
	}
}
