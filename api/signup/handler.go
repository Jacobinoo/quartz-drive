package signup

import (
	"encoding/base64"
	"encoding/json"
	"log"
	"net/http"
	"quartz/config"
	"quartz/internal/bindings"
	"quartz/internal/dto"
	"quartz/internal/model"

	// pb "quartz/proto"

	"github.com/redis/go-redis/v9"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Handler struct {
	db          *gorm.DB
	redis       *redis.Client
	opaqueSetup []byte
}

func NewHandler(db *gorm.DB, redisClient *redis.Client, opaqueSetup []byte) *Handler {
	return &Handler{db: db, redis: redisClient, opaqueSetup: opaqueSetup}
}

// Signup: (opaque receive m1 & send m2)
func (h *Handler) Signup(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var m1 dto.M1
	if err := json.NewDecoder(r.Body).Decode(&m1); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	credID := uuid.New()

	regReqBytes, err := base64.RawURLEncoding.DecodeString(m1.RegistrationRequest)
	if err != nil {
		http.Error(w, "invalid base64 in registration request", http.StatusBadRequest)
		return
	}

	regResponse, err := bindings.StartRegistration(
		h.opaqueSetup,
		regReqBytes,
		[]byte(credID.String()),
	)

	var registrationResponse dto.M2

	if err == nil {
		registrationResponse = dto.M2{
			Status:               "ok",
			RegistrationResponse: base64.RawURLEncoding.EncodeToString(regResponse),
			Nonce:                credID.String(), // Use the generated UUID as the nonce
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

	uuidString := m3.User.APAKE.RegistrationNonce
	credID, uuidParseErr := uuid.Parse(uuidString)
	if uuidParseErr != nil {
		http.Error(w, "invalid user identifier in nonce", http.StatusBadRequest)
		return
	}

	regRecordBytes, err := base64.RawURLEncoding.DecodeString(m3.User.APAKE.RegistrationRecord)
	if err != nil {
		http.Error(w, "invalid base64 in registration record", http.StatusBadRequest)
		return
	}

	passwordFileRecord, err := bindings.FinishRegistration(
		regRecordBytes,
	)

	if err != nil {
		log.Printf("bindings FinishRegistration call failed: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	} else {
		log.Println("bindings FinishRegistration call succeeded")
	}

	m3.User.APAKE.RegistrationRecord = base64.RawURLEncoding.EncodeToString(passwordFileRecord)

	err = h.db.Transaction(func(tx *gorm.DB) error {
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
