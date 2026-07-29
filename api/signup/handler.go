package signup

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"quartz/config"
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
		Nonce:              m3.User.APAKE.RegistrationNonce,
		RegistrationRecord: m3.User.APAKE.RegistrationRecord,
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

	err := h.db.Transaction(func(tx *gorm.DB) error {
		storedUserKeyStore := model.UserKeyStore{
			UserID:                               credID,
			MasterKdfSalt:                        m3.User.Keys.MasterKdfSalt,
			AccountEncryptionPublicKey:           m3.User.Keys.AccountEncryptionPublicKey,
			EncryptedAccountEncryptionPrivateKey: m3.User.Keys.EncryptedAccountEncryptionPrivateKey,
			AccountEncryptionKeyNonce:            m3.User.Keys.AccountEncryptionKeyNonce,
			AccountSigningPublicKey:              m3.User.Keys.AccountSigningPublicKey,
			EncryptedAccountSigningPrivateKey:    m3.User.Keys.EncryptedAccountSigningPrivateKey,
			AccountSigningKeyNonce:               m3.User.Keys.AccountSigningKeyNonce,
		}

		storedUser := model.User{
			ID:                 credID,
			Email:              m3.User.Email,
			RegistrationRecord: m3.User.APAKE.RegistrationRecord,
			RegistrationNonce:  m3.User.APAKE.RegistrationNonce,
			KdfParams: dto.KdfParams{
				KdfAlg:      config.Cfg.CRYPTO.KdfAlg,
				KdfOpsLimit: config.Cfg.CRYPTO.KdfOpsLimit,
				KdfMemLimit: config.Cfg.CRYPTO.KdfMemLimit,
			},
			EncryptionVersion: config.Cfg.CRYPTO.EncryptionVersion,
		}

		shareUUID := uuid.New()
		linkUUID := uuid.New()
		nodeUUID := uuid.New()

		storedShare := model.Share{
			ID:                     shareUUID,
			TargetLinkID:           linkUUID,
			Type:                   "DEFAULT",
			OwnerID:                credID,
			SharePublicKey:         m3.Drive.DefaultShare.PublicKey,
			WrappedSharePrivateKey: m3.Drive.DefaultShare.WrappedPrivateKey,
			SharePrivNonce:         m3.Drive.DefaultShare.PrivKeyNonce,
		}

		storedNode := model.Node{
			ID:                nodeUUID,
			Type:              model.NodeTypeFolder,
			EncryptedMetadata: "",
			MetadataNonce:     "",
			OwnerID:           credID,
			NodePublicKey:     m3.Drive.RootNode.PublicKey,
			WrappedNodeKey:    m3.Drive.RootNode.WrappedPrivateKey,
			NodePrivNonce:     m3.Drive.RootNode.PrivKeyNonce,
			Signature:         m3.Drive.RootNode.SignedEncryptedPassphrase,
		}

		storedLink := model.Link{
			ID:                            linkUUID,
			ParentNodeID:                  nil,
			ChildNodeID:                   &nodeUUID,
			EncryptedName:                 "",
			NameNonce:                     "",
			EncryptedNodePassphrase:       m3.Drive.RootNode.EncryptedPassphrase,
			SignedEncryptedNodePassphrase: m3.Drive.RootNode.SignedEncryptedPassphrase,
			AuthorID:                      credID,
		}

		storedShareMember := model.ShareMember{
			ShareID:                        shareUUID,
			UserID:                         credID,
			Permissions:                    255, // Full Admin
			EncryptedSharePassphrase:       m3.Drive.DefaultShare.EncryptedPassphraseForOwner,
			SignedEncryptedSharePassphrase: m3.Drive.DefaultShare.SignedEncryptedPassphraseForOwner,
		}

		if err := tx.Create(&storedUser).Error; err != nil {
			// registrationSessions.Delete(m3.RegistrationNonce)
			http.Error(w, err.Error(), http.StatusConflict)
			return err
		}

		if err := tx.Create(&storedUserKeyStore).Error; err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return err
		}

		if err := tx.Create(&storedNode).Error; err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return err
		}

		if err := tx.Create(&storedLink).Error; err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return err
		}

		if err := tx.Create(&storedShare).Error; err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return err
		}

		if err := tx.Create(&storedShareMember).Error; err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return err
		}

		return nil
	})

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)

	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
