package signup

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"quartz/internal/dto"
	"quartz/internal/model"
	pb "quartz/proto"

	"github.com/google/uuid"
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

// Signup: (opaque receive m1 & send m2)
func (h *Handler) Signup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var m1 dto.M1
	if err := json.NewDecoder(r.Body).Decode(&m1); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	grpcStartRegistrationReq := &pb.StartRegistrationRequest{
		RegistrationRequest: m1.RegistrationRequest,
		Email:               m1.Email,
	}

	startRegRes, startRegErr := h.grpc.StartRegistration(h.ctx, grpcStartRegistrationReq)
	if startRegErr != nil {
		log.Printf("grpc StartRegistration call failed: %v", startRegErr)
	} else {
		log.Println("grpc StartRegistration call succeeded")
	}

	var registrationResponse dto.M2

	if startRegErr == nil {
		registrationResponse = dto.M2{
			Status:               "ok",
			RegistrationResponse: startRegRes.RegistrationResponse,
			Nonce:                startRegRes.Nonce,
		}
		w.WriteHeader(http.StatusOK)
	} else {
		registrationResponse = dto.M2{
			Status:               "error",
			RegistrationResponse: "",
			Nonce:                "",
		}
		w.WriteHeader(http.StatusBadRequest)
	}

	json.NewEncoder(w).Encode(registrationResponse)
}

// Signup: store new user in DB (opaque receive m3)
func (h *Handler) SignupM3(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var m3 dto.M3

	if err := json.NewDecoder(r.Body).Decode(&m3); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	grpcFinishRegistrationReq := &pb.FinishRegistrationRequest{
		Nonce:              m3.RegistrationNonce,
		RegistrationRecord: m3.RegistrationRecord,
	}

	finishRegRes, finishRegErr := h.grpc.FinishRegistration(h.ctx, grpcFinishRegistrationReq)
	if finishRegErr != nil {
		log.Printf("grpc FinishRegistration call failed: %v", finishRegErr)
	} else {
		log.Println("grpc FinishRegistration call succeeded")
	}

	uuidString := finishRegRes.Uuid
	credID, uuidParseErr := uuid.Parse(uuidString)
	if uuidParseErr != nil {
		http.Error(w, uuidParseErr.Error(), http.StatusInternalServerError)
		return
	}

	storedUser := model.User{
		ID: credID,
		M3: dto.M3{
			Email:               m3.Email,
			Salt:                m3.Salt,
			PublicKey:           m3.PublicKey,
			EncryptedPrivateKey: m3.EncryptedPrivateKey,
			Nonce:               m3.Nonce,
			RegistrationRecord:  m3.RegistrationRecord,
			RegistrationNonce:   m3.RegistrationNonce,
		},
	}

	if err := h.db.Create(&storedUser).Error; err != nil {
		// registrationSessions.Delete(m3.RegistrationNonce)
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	w.WriteHeader(http.StatusCreated)

	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
