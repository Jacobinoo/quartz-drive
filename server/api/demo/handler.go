package demo

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"quartz/config"
	"quartz/internal/dto"
	"quartz/internal/model"
	apperrors "quartz/pkg/app-errors"
	"quartz/pkg/captcha"
	"quartz/pkg/contextkeys"
	crypto "quartz/pkg/crypto"
	"quartz/pkg/discord"
	"quartz/pkg/dpop"
	"quartz/pkg/storage"
	"quartz/pkg/token"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/mssola/useragent"
	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/errgroup"
	"gorm.io/gorm"
)

type Handler struct {
	db          *gorm.DB
	redis       *redis.Client
	asynqClient *asynq.Client
	storage     storage.StorageService
}

func NewHandler(db *gorm.DB, redisClient *redis.Client, asynqClient *asynq.Client, storageService storage.StorageService) *Handler {
	return &Handler{db: db, redis: redisClient, asynqClient: asynqClient, storage: storageService}
}

func (h *Handler) DemoStart(w http.ResponseWriter, r *http.Request) error {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		return apperrors.NewMethodNotAllowed("method not allowed")
	}

	if h.redis.Get(r.Context(), "admin:disable_demo").Val() == "true" {
		return apperrors.NewForbidden("Demo is currently disabled. Please check back later.", nil)
	}

	success, errArr, err := captcha.VerifyTurnstileTokenInRequest(r)
	if err != nil {
		slog.WarnContext(r.Context(), "turnstile verification failed", "errors", strings.Join(errArr, ","), "error", err)
		return apperrors.NewBadRequest("invalid token", err)
	}
	if !success {
		slog.WarnContext(r.Context(), "turnstile verification failed", "errors", strings.Join(errArr, ","))
		return apperrors.NewBadRequest("invalid token", nil)
	}

	dpopHeader := r.Header.Get("DPoP")
	thumbprint, err := dpop.ValidateDpopProof(dpopHeader, r)
	if err != nil {
		slog.InfoContext(r.Context(), "invalid dpop proof", "error", err)
		return apperrors.NewBadRequest("invalid dpop proof", err)
	}

	userId := uuid.New()
	email := fmt.Sprintf("demo-account-quartz-%s@quartz.local", userId.String())

	hashedEmailHex, err := crypto.HashEmail([]byte(email))
	if err != nil {
		return apperrors.NewInternal(err)
	}

	encryptedEmail, err := crypto.EncryptEmail([]byte(email))
	if err != nil {
		return apperrors.NewInternal(err)
	}

	var trustedUserInfo dto.TrustedUserInformation

	err = h.db.Transaction(func(tx *gorm.DB) error {
		storedUser := model.User{
			ID:                 userId,
			EncryptedEmail:     encryptedEmail,
			HashedEmail:        hashedEmailHex,
			RegistrationRecord: config.Cfg.Demo.RegistrationRecord,
			EncryptionVersion:  config.Cfg.CRYPTO.EncryptionVersion,
			KdfParams: dto.KdfParams{
				KdfAlg:      config.Cfg.CRYPTO.KdfAlg,
				KdfOpsLimit: config.Cfg.CRYPTO.KdfOpsLimit,
				KdfMemLimit: config.Cfg.CRYPTO.KdfMemLimit,
			},
			IsDemo: true,
		}

		storedUserKeyStore := model.UserKeyStore{
			UserID:                               userId,
			MasterKdfSalt:                        config.Cfg.Demo.MasterKdfSalt,
			AccountEncryptionPublicKey:           config.Cfg.Demo.AccountEncryptionPublicKey,
			EncryptedAccountEncryptionPrivateKey: config.Cfg.Demo.EncryptedAccountEncryptionPrivateKey,
			AccountEncryptionKeyNonce:            config.Cfg.Demo.AccountEncryptionKeyNonce,
			AccountSigningPublicKey:              config.Cfg.Demo.AccountSigningPublicKey,
			EncryptedAccountSigningPrivateKey:    config.Cfg.Demo.EncryptedAccountSigningPrivateKey,
			AccountSigningKeyNonce:               config.Cfg.Demo.AccountSigningKeyNonce,
		}

		shareUUID := uuid.New()
		linkUUID := uuid.New()
		nodeUUID := uuid.New()

		storedShare := model.Share{
			ID:                     shareUUID,
			TargetLinkID:           linkUUID,
			Type:                   "DEFAULT",
			OwnerID:                userId,
			SharePublicKey:         config.Cfg.Demo.DefaultSharePublicKey,
			WrappedSharePrivateKey: config.Cfg.Demo.DefaultShareWrappedPrivateKey,
			SharePrivNonce:         config.Cfg.Demo.DefaultSharePrivateKeyNonce,
		}

		volumeUUID := uuid.New()

		storedVolume := model.Volume{
			ID:           volumeUUID,
			Type:         model.VolumeTypePrivate,
			OwnerUserID:  &userId,
			RootNodeID:   nodeUUID,
			StorageQuota: 104857600, // 100MiB default
			StorageUsed:  0,
		}

		storedNode := model.Node{
			ID:                nodeUUID,
			Type:              model.NodeTypeFolder,
			EncryptedMetadata: "",
			MetadataNonce:     "",
			OwnerID:           userId,
			VolumeID:          volumeUUID,
			NodePublicKey:     config.Cfg.Demo.RootNodePublicKey,
			WrappedNodeKey:    config.Cfg.Demo.RootNodeWrappedPrivateKey,
			NodePrivNonce:     config.Cfg.Demo.RootNodePrivKeyNonce,
			Signature:         config.Cfg.Demo.RootNodeSignedEncryptedPassphrase,
		}

		storedLink := model.Link{
			ID:                            linkUUID,
			ParentNodeID:                  nil,
			ChildNodeID:                   &nodeUUID,
			EncryptedName:                 "",
			NameNonce:                     "",
			EncryptedNodePassphrase:       config.Cfg.Demo.RootNodeEncryptedPassphrase,
			SignedEncryptedNodePassphrase: config.Cfg.Demo.RootNodeSignedEncryptedPassphrase,
			AuthorID:                      userId,
		}

		storedShareMember := model.ShareMember{
			ShareID:                        shareUUID,
			UserID:                         userId,
			Permissions:                    255,
			EncryptedSharePassphrase:       config.Cfg.Demo.DefaultShareEncryptedPassphraseForOwner,
			SignedEncryptedSharePassphrase: config.Cfg.Demo.DefaultShareSignedEncryptedPassphraseForOwner,
		}

		if err := tx.Create(&storedUser).Error; err != nil {
			return fmt.Errorf("cannot initialize user: %w", err)
		}

		if err := tx.Create(&storedUserKeyStore).Error; err != nil {
			return fmt.Errorf("cannot initialize keys: %w", err)
		}

		if err := tx.Create(&storedVolume).Error; err != nil {
			return fmt.Errorf("cannot initialize volume: %w", err)
		}

		if err := tx.Create(&storedNode).Error; err != nil {
			return fmt.Errorf("cannot initialize node: %w", err)
		}

		if err := tx.Create(&storedLink).Error; err != nil {
			return fmt.Errorf("cannot initialize link: %w", err)
		}

		if err := tx.Create(&storedShare).Error; err != nil {
			return fmt.Errorf("cannot initialize share: %w", err)
		}

		if err := tx.Create(&storedShareMember).Error; err != nil {
			return fmt.Errorf("cannot initialize share member: %w", err)
		}

		trustedUserInfo = dto.TrustedUserInformation{
			ID:    userId,
			Email: email,
			KdfParams: dto.KdfParams{
				KdfAlg:      config.Cfg.CRYPTO.KdfAlg,
				KdfOpsLimit: config.Cfg.CRYPTO.KdfOpsLimit,
				KdfMemLimit: config.Cfg.CRYPTO.KdfMemLimit,
			},
			EncryptionVersion:                    config.Cfg.CRYPTO.EncryptionVersion,
			MasterKdfSalt:                        config.Cfg.Demo.MasterKdfSalt,
			AccountEncryptionPublicKey:           config.Cfg.Demo.AccountEncryptionPublicKey,
			EncryptedAccountEncryptionPrivateKey: config.Cfg.Demo.EncryptedAccountEncryptionPrivateKey,
			AccountEncryptionKeyNonce:            config.Cfg.Demo.AccountEncryptionKeyNonce,
			AccountSigningPublicKey:              config.Cfg.Demo.AccountSigningPublicKey,
			EncryptedAccountSigningPrivateKey:    config.Cfg.Demo.EncryptedAccountSigningPrivateKey,
			AccountSigningKeyNonce:               config.Cfg.Demo.AccountSigningKeyNonce,
		}

		if err := cloneTemplateTree(r.Context(), tx, h.storage, nodeUUID, userId, volumeUUID); err != nil {
			return fmt.Errorf("failed to clone showcase files: %w", err)
		}

		return nil
	})

	if err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return apperrors.NewConflict("this demo account already exists", err)
		}
		if errors.Is(err, context.Canceled) || strings.Contains(err.Error(), "context canceled") {
			return apperrors.NewBadRequest("request cancelled", err)
		}

		slog.ErrorContext(r.Context(), "failed to initialize demo account", "error", err)
		return apperrors.NewInternal(err)
	}

	uaHeader := r.Header.Get("User-Agent")
	if uaHeader == "" {
		return apperrors.NewBadRequest("invalid user agent", nil)
	}
	ua := useragent.New(uaHeader)
	name, _ := ua.Browser()
	displayedDeviceName := ua.OS() + " - " + ua.Model() + " - " + name

	generatedNewSessionID := uuid.New()

	newSessionEntry := model.Session{
		ID:           generatedNewSessionID,
		UserID:       userId,
		UserAgent:    uaHeader,
		DeviceName:   displayedDeviceName,
		LastActiveAt: time.Now(),
	}

	familyID := uuid.New()
	refreshToken := token.IssueRefreshToken()
	newCsrfToken := token.IssueCsrfToken()

	const transientRefreshTokenLifetime = 1 * time.Hour

	expiresAt := time.Now().Add(transientRefreshTokenLifetime)

	refreshTokenEntry := model.GormRefreshToken{
		UserID:        userId,
		TokenHash:     refreshToken.TokenSha256Hash,
		FamilyID:      familyID,
		IsRevoked:     false,
		ExpiresAt:     expiresAt,
		CsrfTokenHash: newCsrfToken.TokenSha256Hash,
		Session:       newSessionEntry,
		DpopJKT:       thumbprint,
	}

	fingerprintBytes := make([]byte, 64)
	_, _ = rand.Read(fingerprintBytes)
	fingerprint := hex.EncodeToString(fingerprintBytes)

	fingerprintHashBytes := sha256.Sum256(fingerprintBytes)
	fingerprintHash := hex.EncodeToString(fingerprintHashBytes[:])

	accessToken, expTime := token.IssueAccessToken(true, fingerprintHash, thumbprint, userId.String(), email, generatedNewSessionID.String(), familyID.String(), true, expiresAt.Unix())
	maxAge := time.Until(expTime)

	http.SetCookie(w, &http.Cookie{
		Name:     "__Secure-F",
		Value:    fingerprint,
		HttpOnly: true,
		Path:     "/",
		MaxAge:   int(maxAge.Seconds()),
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})

	http.SetCookie(w, &http.Cookie{
		Name:     "__Secure-Auth",
		Value:    refreshToken.Token,
		HttpOnly: true,
		Path:     "/",
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		Expires:  expiresAt,
		MaxAge:   int(transientRefreshTokenLifetime.Seconds()),
	})

	if err := h.db.Create(&refreshTokenEntry).Error; err != nil {
		return apperrors.NewInternal(err)
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

	discord.Notify(r.Context(), "Demo User Signed In", "New demo session started! "+email, 3066993)

	w.WriteHeader(http.StatusCreated)
	err = json.NewEncoder(w).Encode(loginTrustAttestation)
	if err != nil {
		return apperrors.NewInternal(err)
	}

	return nil
}

func (h *Handler) DemoFinish(w http.ResponseWriter, r *http.Request) error {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		return apperrors.NewMethodNotAllowed("method not allowed")
	}

	userIdStr, ok := r.Context().Value(contextkeys.UserIDKey).(uuid.UUID)
	if !ok || userIdStr.String() == "" {
		return apperrors.NewUnauthorized("unauthorized", nil)
	}

	userId := userIdStr

	var user model.User
	if err := h.db.First(&user, "id = ?", userId).Error; err != nil {
		return apperrors.NewUnauthorized("user not found", err)
	}

	if !user.IsDemo {
		return apperrors.NewForbidden("not a demo account", nil)
	}

	go func() {
		err := h.cleanupDemoUser(context.Background(), userId)
		if err != nil {
			slog.Error("failed to fully clean up demo user asynchronously", "userId", userId.String(), "error", err)
			// proceed to wipe cookies anyway, the sweeper will finish it later.
		}
	}()

	http.SetCookie(w, &http.Cookie{
		Name:     "__Secure-Auth",
		Value:    "",
		HttpOnly: true,
		Path:     "/",
		MaxAge:   -1,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})

	http.SetCookie(w, &http.Cookie{
		Name:     "__Secure-F",
		Value:    "",
		HttpOnly: true,
		Path:     "/",
		MaxAge:   -1,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})

	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	return nil
}

func (h *Handler) cleanupDemoUser(ctx context.Context, userID uuid.UUID) error {
	var files []model.Node
	if err := h.db.Where("owner_id = ? AND type = 'FILE'", userID).Find(&files).Error; err != nil {
		return err
	}

	var nodeIDs []string
	for _, f := range files {
		nodeIDs = append(nodeIDs, f.ID.String())
	}

	var objectKeys []string

	if len(nodeIDs) > 0 {
		var fileBlocks []model.FileBlock
		h.db.Where("node_id IN ?", nodeIDs).Find(&fileBlocks)
		for _, b := range fileBlocks {
			if b.ObjectKey != "" {
				objectKeys = append(objectKeys, b.ObjectKey)
			}
		}

		var uploads []model.Upload
		h.db.Where("node_id IN ?", nodeIDs).Find(&uploads)

		var uploadIDs []string
		for _, u := range uploads {
			uploadIDs = append(uploadIDs, u.ID.String())
		}

		if len(uploadIDs) > 0 {
			var uploadChunks []model.UploadChunk
			h.db.Where("upload_id IN ?", uploadIDs).Find(&uploadChunks)
			for _, c := range uploadChunks {
				if c.ObjectKey != "" {
					objectKeys = append(objectKeys, c.ObjectKey)
				}
			}
		}
	}

	s3Success := true
	for _, key := range objectKeys {
		if err := h.storage.DeleteChunk(ctx, key); err != nil {
			if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "NoSuchKey") {
				continue
			}
			slog.ErrorContext(ctx, "failed to wipe s3 chunk during demo cleanup", "key", key, "error", err)
			s3Success = false
		}
	}

	if !s3Success {
		return fmt.Errorf("failed to fully wipe S3 objects, skipping DB deletion so the sweeper can retry")
	}

	return h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("DELETE FROM share_members WHERE user_id = ? OR share_id IN (SELECT id FROM shares WHERE owner_id = ?)", userID, userID).Error; err != nil {
			return err
		}
		if err := tx.Exec("DELETE FROM shares WHERE owner_id = ?", userID).Error; err != nil {
			return err
		}
		if err := tx.Exec("DELETE FROM links WHERE author_id = ?", userID).Error; err != nil {
			return err
		}
		if len(nodeIDs) > 0 {
			if err := tx.Exec("DELETE FROM file_blocks WHERE node_id IN ?", nodeIDs).Error; err != nil {
				return err
			}
		}
		if err := tx.Exec("DELETE FROM upload_chunks WHERE upload_id IN (SELECT id FROM uploads WHERE user_id = ?)", userID).Error; err != nil {
			return err
		}
		if err := tx.Exec("DELETE FROM uploads WHERE user_id = ?", userID).Error; err != nil {
			return err
		}
		if err := tx.Exec("DELETE FROM nodes WHERE owner_id = ?", userID).Error; err != nil {
			return err
		}
		if err := tx.Exec("DELETE FROM volumes WHERE owner_user_id = ?", userID).Error; err != nil {
			return err
		}
		if err := tx.Exec("DELETE FROM user_key_stores WHERE user_id = ?", userID).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Delete(&model.User{}, "id = ?", userID).Error; err != nil {
			return err
		}

		return nil
	})
}

func cloneTemplateTree(ctx context.Context, tx *gorm.DB, storageSvc storage.StorageService, newRootNodeId uuid.UUID, newOwnerId uuid.UUID, newVolumeId uuid.UUID) error {
	templateUserId, err := uuid.Parse("00000000-0000-0000-0000-000000000000")
	if err != nil {
		return err
	}

	if templateUserId == newOwnerId {
		return nil // Avoid cloning into itself when setting up the template user
	}

	var templateVolume model.Volume
	if err := tx.Where("owner_user_id = ?", templateUserId).First(&templateVolume).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	templateRootNodeId := templateVolume.RootNodeID

	nodeMap := make(map[uuid.UUID]uuid.UUID)
	nodeMap[templateRootNodeId] = newRootNodeId

	var templateNodes []model.Node
	if err := tx.Where("owner_id = ? AND id != ?", templateUserId, templateRootNodeId).Find(&templateNodes).Error; err != nil {
		return err
	}

	for _, n := range templateNodes {
		newId := uuid.New()
		nodeMap[n.ID] = newId

		newNode := n
		newNode.ID = newId
		newNode.OwnerID = newOwnerId
		newNode.VolumeID = newVolumeId

		if err := tx.Create(&newNode).Error; err != nil {
			return err
		}
	}

	var templateLinks []model.Link
	if err := tx.Where("author_id = ?", templateUserId).Find(&templateLinks).Error; err != nil {
		return err
	}

	for _, l := range templateLinks {
		// Skip root node link of the template user
		if l.ParentNodeID == nil && l.ChildNodeID != nil && *l.ChildNodeID == templateRootNodeId {
			continue
		}

		newId := uuid.New()
		newLink := l
		newLink.ID = newId
		newLink.AuthorID = newOwnerId

		if l.ParentNodeID != nil {
			newParentId, ok := nodeMap[*l.ParentNodeID]
			if !ok {
				continue
			}
			newLink.ParentNodeID = &newParentId
		}

		if l.ChildNodeID != nil {
			newChildId, ok := nodeMap[*l.ChildNodeID]
			if !ok {
				continue
			}
			newLink.ChildNodeID = &newChildId
		}

		if err := tx.Create(&newLink).Error; err != nil {
			return err
		}
	}

	var templateBlocks []model.FileBlock
	var nodeIds []uuid.UUID
	for _, n := range templateNodes {
		if n.Type == model.NodeTypeFile {
			nodeIds = append(nodeIds, n.ID)
		}
	}

	var copiedS3Keys []string

	if len(nodeIds) > 0 {
		if err := tx.Where("node_id IN ?", nodeIds).Find(&templateBlocks).Error; err != nil {
			return err
		}

		type blockCopyTask struct {
			srcKey   string
			destKey  string
			newBlock model.FileBlock
		}

		tasks := make([]blockCopyTask, len(templateBlocks))
		for i, b := range templateBlocks {
			newNodeId := nodeMap[b.NodeID]
			newObjectKey := uuid.New().String()

			newBlock := b
			originalNodeId := b.NodeID // preserve the original for AEAD AD
			newBlock.ID = uuid.New()
			newBlock.NodeID = newNodeId
			newBlock.ObjectKey = newObjectKey
			newBlock.EncryptingNodeID = &originalNodeId

			newBlock.CreatedAt = time.Time{}
			newBlock.UpdatedAt = time.Time{}
			newBlock.DeletedAt = gorm.DeletedAt{}

			tasks[i] = blockCopyTask{
				srcKey:   b.ObjectKey,
				destKey:  newObjectKey,
				newBlock: newBlock,
			}
		}

		var copiedMu sync.Mutex
		g, gCtx := errgroup.WithContext(ctx)
		g.SetLimit(16) // Concurrently copy up to 16 chunks at a time

		for _, task := range tasks {
			t := task
			g.Go(func() error {
				if err := storageSvc.CopyChunk(gCtx, t.srcKey, t.destKey); err != nil {
					return err
				}
				copiedMu.Lock()
				copiedS3Keys = append(copiedS3Keys, t.destKey)
				copiedMu.Unlock()
				return nil
			})
		}

		if err := g.Wait(); err != nil {
			if len(copiedS3Keys) > 0 {
				go func(keysToSweep []string) {
					for _, key := range keysToSweep {
						_ = storageSvc.DeleteChunk(context.Background(), key)
					}
				}(copiedS3Keys)
			}
			return err
		}

		newBlocks := make([]model.FileBlock, len(tasks))
		for i, t := range tasks {
			newBlocks[i] = t.newBlock
		}

		if len(newBlocks) > 0 {
			if err := tx.CreateInBatches(&newBlocks, 100).Error; err != nil {
				go func(keysToSweep []string) {
					for _, key := range keysToSweep {
						_ = storageSvc.DeleteChunk(context.Background(), key)
					}
				}(copiedS3Keys)
				return err
			}
		}
	}

	var totalSize int64
	for _, b := range templateBlocks {
		totalSize += int64(b.Size)
	}
	if totalSize > 0 {
		if err := tx.Model(&model.Volume{}).Where("id = ?", newVolumeId).
			Update("storage_used", totalSize).Error; err != nil {

			go func(keysToSweep []string) {
				for _, key := range keysToSweep {
					_ = storageSvc.DeleteChunk(context.Background(), key)
				}
			}(copiedS3Keys)
			return err
		}
	}

	return nil
}

func (h *Handler) DemoLock(w http.ResponseWriter, r *http.Request) error {
	templateUserId := uuid.MustParse("00000000-0000-0000-0000-000000000000")

	var templateVolume model.Volume
	if err := h.db.Where("owner_user_id = ?", templateUserId).First(&templateVolume).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.NewBadRequest("Template user not found. Please upload files to it first.", nil)
		}
		return err
	}

	var templateNodes []model.Node
	if err := h.db.Where("owner_id = ? AND type = ?", templateUserId, model.NodeTypeFile).Find(&templateNodes).Error; err != nil {
		return err
	}

	var nodeIds []uuid.UUID
	for _, n := range templateNodes {
		nodeIds = append(nodeIds, n.ID)
	}

	if len(nodeIds) == 0 {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("No files found for template user. Nothing to lock."))
		return nil
	}

	var templateBlocks []model.FileBlock
	if err := h.db.Where("node_id IN ?", nodeIds).Find(&templateBlocks).Error; err != nil {
		return err
	}

	ctx := r.Context()
	lockedCount := 0
	for _, block := range templateBlocks {
		err := h.storage.ApplyLegalHold(ctx, block.ObjectKey, true)
		if err != nil {
			slog.ErrorContext(ctx, "Failed to apply legal hold", "objectKey", block.ObjectKey, "error", err)
		} else {
			lockedCount++
		}
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(fmt.Sprintf("Finished. Successfully locked %d/%d chunks.", lockedCount, len(templateBlocks))))
	return nil
}
