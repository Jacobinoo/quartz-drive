package files

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"math"
	"net/http"
	"quartz/config"
	"quartz/internal/dto"
	"quartz/internal/model"
	apperrors "quartz/pkg/app-errors"
	"quartz/pkg/contextkeys"
	"quartz/pkg/storage"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type Handler struct {
	db      *gorm.DB
	storage storage.StorageService
	rdb     *redis.Client
}

func NewHandler(db *gorm.DB, storage storage.StorageService, rdb *redis.Client) *Handler {
	return &Handler{db: db, storage: storage, rdb: rdb}
}

// inits upload session and sends back upload & node id
func (h *Handler) InitUpload(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodPost {
		return apperrors.NewMethodNotAllowed("This method is not supported")
	}

	w.Header().Set("Content-Type", "application/json")

	var uploadRequest dto.InitFileUploadRequest
	if err := json.NewDecoder(r.Body).Decode(&uploadRequest); err != nil {
		return apperrors.NewBadRequest("invalid request", err)
	}

	userID := r.Context().Value(contextkeys.UserIDKey)
	if userID == nil {
		return apperrors.NewUnauthorized("access token invalid", nil)
	}

	if uploadRequest.TotalChunks <= 0 || uploadRequest.TotalFileSize <= 0 {
		return apperrors.NewValidation("total chunks or total file size invalid", nil)
	}

	const maxChunkSize = 4 * 1024 * 1024 //4MB

	parentUUID, err := uuid.Parse(uploadRequest.ParentNodeID)
	if err != nil {
		return apperrors.NewBadRequest("invalid parentNodeId uuid", nil)
	}

	var parentNode model.Node
	if err := h.db.First(&parentNode, "id = ? AND owner_id = ?", parentUUID, userID).Error; err != nil {
		return apperrors.NewNotFound("parent folder not found", nil)
	}

	var user model.User
	if err := h.db.First(&user, "id = ?", userID).Error; err != nil {
		return apperrors.NewUnauthorized("user not found", nil)
	}

	if user.StorageUsed+uploadRequest.TotalFileSize > user.StorageQuota {
		return apperrors.NewForbidden("quota exceeded", nil)
	}

	minPlausibleChunks := int(math.Ceil(float64(uploadRequest.TotalFileSize) / float64(maxChunkSize)))
	if uploadRequest.TotalChunks < int64(minPlausibleChunks) {
		return apperrors.NewValidation("totalChunks too low for declared file size", nil)
	}

	const chunkCountSlack = 1.05 // 5% slack for per-chunk overhead
	maxPlausibleChunks := int(math.Ceil(float64(minPlausibleChunks)*chunkCountSlack)) + 1
	if uploadRequest.TotalChunks > int64(maxPlausibleChunks) {
		return apperrors.NewValidation("totalChunks too high for declared file size", nil)
	}

	nodeID, nErr := uuid.NewV7()
	uploadID, uErr := uuid.NewV7()
	if nErr != nil || uErr != nil {
		return apperrors.NewInternal(fmt.Errorf("internal server error"))
	}
	expiresAt := time.Now().UTC().Add(time.Duration(config.Cfg.Sweeper.UploadSessionExpiresHours) * time.Hour)

	chunkRows := []model.UploadChunk{}

	for i := int64(0); i < uploadRequest.TotalChunks; i++ {
		id, err := uuid.NewV7()
		if err != nil {
			return apperrors.NewInternal(err)
		}

		chunkRows = append(chunkRows, model.UploadChunk{
			ID:         id,
			UploadID:   uploadID,
			ChunkIndex: i,
			ObjectKey:  fmt.Sprintf("%s/authors/%s/uploads/%s/chunk_%d", nodeID.String(), user.ID.String(), uploadID.String(), i),
			Status:     model.ChunkUploadStatusPending,
		})
	}

	err = h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&model.Upload{
			ID:                uploadID,
			UserID:            user.ID,
			NodeID:            nodeID,
			TotalChunks:       uploadRequest.TotalChunks,
			ReportedTotalSize: uploadRequest.TotalFileSize,
			Status:            model.UploadStatusPending,
			ExpiresAt:         expiresAt,

			ParentNodeID:                  &parentUUID,
			EncryptedName:                 uploadRequest.EncryptedName,
			NameNonce:                     uploadRequest.NameNonce,
			EncryptedNodePassphrase:       uploadRequest.EncryptedNodePassphrase,
			SignedEncryptedNodePassphrase: uploadRequest.SignedEncryptedNodePassphrase,
			NodePublicKey:                 uploadRequest.NodePublicKey,
			WrappedNodeKey:                uploadRequest.WrappedNodeKey,
			NodePrivNonce:                 uploadRequest.NodePrivNonce,
			HasChildren:                   uploadRequest.HasChildren,

			EncryptedMetadata: uploadRequest.EncryptedMetadata,
			MetadataNonce:     uploadRequest.MetadataNonce,
		}).Error; err != nil {
			return err
		}

		slog.Debug("chunkRows", "chunk_rows_amount", len(chunkRows))

		if err := tx.CreateInBatches(chunkRows, len(chunkRows)).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return apperrors.NewInternal(fmt.Errorf("could not init an upload session"))
	}

	uploadResponse := InitFileUploadResponse{
		UploadID:  uploadID,
		NodeID:    nodeID,
		ExpiresAt: expiresAt,
	}

	err = json.NewEncoder(w).Encode(uploadResponse)
	if err != nil {
		return apperrors.NewInternal(fmt.Errorf("internal server error"))
	}
	return nil
}

// validates chunk upload request and presigns an url for that chunk
func (h *Handler) Upload(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodPost {
		return apperrors.NewMethodNotAllowed("method not allowed")
	}

	w.Header().Set("Content-Type", "application/json")

	var uploadRequest dto.RequestChunkUploadRequest
	if err := json.NewDecoder(r.Body).Decode(&uploadRequest); err != nil {
		slog.Debug("failed to decode RequestChunkUploadRequest")
		return apperrors.NewBadRequest("invalid request", err)
	}

	userID := r.Context().Value(contextkeys.UserIDKey)
	if userID == nil {
		slog.InfoContext(r.Context(), "access token invalid")
		return apperrors.NewUnauthorized("access token invalid", nil)
	}

	const maxChunkSize = 4*1024*1024 + 64 // 4MiB + small margin for AEAD overhead/headers
	const expiry = 2 * time.Minute

	// check if the user exists
	// check if the upload session exists
	// check if the userID from access token is the owner of upload session
	// check if the upload_chunks chunk of the requested index is pending (if not, abort)
	// validate the chunk size
	// generate a presigned put url signed with Content-Length=req.DeclaredSize using PresignHeader

	var user model.User
	if err := h.db.First(&user, "id = ?", userID).Error; err != nil {
		slog.WarnContext(r.Context(), "user not found")
		return apperrors.NewUnauthorized("user not found", nil)
	}

	var uploadSession model.Upload
	if err := h.db.First(&uploadSession, "id = ?", uploadRequest.UploadID).Error; err != nil {
		slog.WarnContext(r.Context(), "upload session not found")
		return apperrors.NewForbidden("upload session not found", nil)
	}

	if user.ID != uploadSession.UserID {
		slog.WarnContext(r.Context(), "user is not the owner of the upload session")
		return apperrors.NewForbidden("upload session not found", nil)
	}

	if !time.Now().Before(uploadSession.ExpiresAt) {
		slog.WarnContext(r.Context(), "upload session expired")
		return apperrors.NewForbidden("upload session not found", nil)
	}

	if uploadSession.Status != model.UploadStatusPending {
		slog.WarnContext(r.Context(), "upload session is not pending", "status", uploadSession.Status)
		return apperrors.NewForbidden("upload session is no longer active", nil)
	}

	if uploadRequest.DeclaredSize <= 0 || uploadRequest.DeclaredSize > maxChunkSize {
		slog.WarnContext(r.Context(), "declared chunk size out of allowed range")
		return apperrors.NewValidation("declared chunk size out of allowed range", nil)
	}

	if int64(uploadRequest.ChunkIndex) >= uploadSession.TotalChunks || uploadRequest.ChunkIndex < 0 {
		slog.WarnContext(r.Context(), "requested chunk index out of allowed range")
		return apperrors.NewForbidden("requested chunk index out of allowed range", nil)
	}

	if user.StorageUsed+uploadRequest.DeclaredSize > user.StorageQuota {
		slog.InfoContext(r.Context(), "quota exceeded")
		return apperrors.NewForbidden("quota exceeded", nil)
	}

	var chunkRow model.UploadChunk
	if err := h.db.First(&chunkRow, "upload_id = ? AND chunk_index = ?", uploadRequest.UploadID, uploadRequest.ChunkIndex).Error; err != nil {
		slog.WarnContext(r.Context(), "failed to find chunk row %s", err)
		return apperrors.NewForbidden("requested chunk not found", nil)
	}

	if chunkRow.Status != model.ChunkUploadStatusPending {
		slog.WarnContext(r.Context(), "error: requested chunk status", "chunk_idx", chunkRow.ChunkIndex, "chunk_status", chunkRow.Status)
		return apperrors.NewForbidden("requested chunk not found", nil)
	}

	url, err := h.storage.GenerateUploadUrl(r.Context(), chunkRow.ObjectKey, expiry, uploadRequest.DeclaredSize, uploadRequest.ChunkHash)
	if err != nil {
		return apperrors.NewInternal(fmt.Errorf("failed to generate presigned upload url: %w", err))
	}

	chunkRow.GeneratedUrls++
	chunkRow.DeclaredSize = uploadRequest.DeclaredSize

	result := h.db.Save(&chunkRow)
	if result.Error != nil {
		return apperrors.NewInternal(fmt.Errorf("chunk row generated urls amount and declared size could not be saved: %w", result.Error))
	}

	uploadResponse := dto.RequestChunkUploadResponse{
		URL: url,
	}

	err = json.NewEncoder(w).Encode(uploadResponse)
	if err != nil {
		return apperrors.NewInternal(err)
	}
	return nil
}

func (h *Handler) ReportChunkUploadDone(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodPost {
		return apperrors.NewMethodNotAllowed("method not allowed")
	}

	w.Header().Set("Content-Type", "application/json")

	var uploadRequest dto.FinishChunkUploadRequest
	if err := json.NewDecoder(r.Body).Decode(&uploadRequest); err != nil {
		slog.WarnContext(r.Context(), "failed to decode FinishChunkUploadRequest")
		return apperrors.NewBadRequest("invalid request", err)
	}

	userID := r.Context().Value(contextkeys.UserIDKey)
	if userID == nil {
		slog.WarnContext(r.Context(), "access token invalid")
		return apperrors.NewUnauthorized("access token invalid", nil)
	}

	const maxChunkSize = 4*1024*1024 + 64 // 4MiB + small margin for AEAD overhead/headers

	var user model.User
	if err := h.db.First(&user, "id = ?", userID).Error; err != nil {
		slog.WarnContext(r.Context(), "user not found", "queried_user_id", userID, "error", err)
		return apperrors.NewUnauthorized("user not found", nil)
	}

	var uploadSession model.Upload
	if err := h.db.First(&uploadSession, "id = ?", uploadRequest.UploadID).Error; err != nil {
		slog.WarnContext(r.Context(), "upload session not found", "queried_upload_id", uploadRequest.UploadID, "error", err)
		return apperrors.NewForbidden("upload session not found", nil)
	}

	uploadLogger := slog.With(
		"upload_id", uploadRequest.UploadID,
		"chunk_idx", uploadRequest.ChunkIndex,
	)

	if user.ID != uploadSession.UserID {
		uploadLogger.WarnContext(r.Context(), "user is not the owner of the upload session")
		return apperrors.NewForbidden("upload session not found", nil)
	}

	if !time.Now().Before(uploadSession.ExpiresAt) {
		uploadLogger.WarnContext(r.Context(), "upload session expired")
		return apperrors.NewForbidden("upload session not found", nil)
	}

	if int64(uploadRequest.ChunkIndex) >= uploadSession.TotalChunks || uploadRequest.ChunkIndex < 0 {
		uploadLogger.WarnContext(r.Context(), "requested chunk index out of allowed range")
		return apperrors.NewForbidden("requested chunk index out of allowed range", nil)
	}

	var chunkRow model.UploadChunk
	if err := h.db.First(&chunkRow, "upload_id = ? AND chunk_index = ?", uploadRequest.UploadID, uploadRequest.ChunkIndex).Error; err != nil {
		uploadLogger.WarnContext(r.Context(), "failed to find chunk row")
		return apperrors.NewForbidden("requested chunk not found", nil)
	}

	if chunkRow.Status != model.ChunkUploadStatusPending && chunkRow.Status != model.ChunkUploadStatusVerified {
		uploadLogger.WarnContext(r.Context(), "requested chunk status", "status", chunkRow.Status)
		return apperrors.NewNotFound("requested chunk not found", nil)
	}

	// If it's already verified from a previous interrupted attempt, skip S3 validation and go straight to assembly check
	if chunkRow.Status == model.ChunkUploadStatusPending {
		size, etag, sha256, err := h.storage.GetChunkSize(r.Context(), chunkRow.ObjectKey)
		if err != nil {
			wrappedErr := fmt.Errorf("failed to get chunk size from s3 (upload_id: %s, chunk_idx: %d): %w", uploadRequest.UploadID, uploadRequest.ChunkIndex, err)
			return apperrors.NewInternal(wrappedErr)
		}

		uploadLogger.Debug("checksum", "sha256", sha256)

		if subtle.ConstantTimeCompare([]byte(sha256), []byte(uploadRequest.ChunkHash)) == 0 {
			uploadLogger.WarnContext(r.Context(), "integrity check failed", "error", err)
			go func(objectKey string) {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				if delErr := h.storage.DeleteChunk(ctx, objectKey); delErr != nil {
					uploadLogger.WarnContext(r.Context(), "failed to delete chunk (integrity check failed)", "objectKey", objectKey, "error", delErr)
				}
			}(chunkRow.ObjectKey)
			return apperrors.NewForbidden("integrity check failed", err)
		}

		if size > maxChunkSize {
			uploadLogger.WarnContext(r.Context(), "CHUNK OVERSIZED", "size", size, "max_size", maxChunkSize, "object_key", chunkRow.ObjectKey)
			go func(objectKey string) {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				if delErr := h.storage.DeleteChunk(ctx, objectKey); delErr != nil {
					uploadLogger.WarnContext(r.Context(), "failed to delete oversized chunk", "objectKey", objectKey, "error", delErr)
				}
			}(chunkRow.ObjectKey)

			chunkRow.Status = model.ChunkUploadStatusFlagged
			h.db.Save(&chunkRow)
			uploadSession.Status = model.UploadStatusFlaggedMalicious
			h.db.Save(&uploadSession)

			wrappedErr := fmt.Errorf("chunk is oversized (upload_id: %s, chunk_idx: %d, ): %w", uploadRequest.UploadID, uploadRequest.ChunkIndex, err)
			return apperrors.NewValidation("chunk size is invalid", wrappedErr)
		}

		if etag != uploadRequest.Etag {
			uploadLogger.WarnContext(r.Context(), "CHUNK etags dont match", "object_key", chunkRow.ObjectKey, "s3_etag", etag, "provided_etag", uploadRequest.Etag)

			go func(objectKey string) {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				if delErr := h.storage.DeleteChunk(ctx, objectKey); delErr != nil {
					uploadLogger.WarnContext(r.Context(), "failed to delete chunk (mismatch etag)", "object_key", objectKey, "error", delErr)
				}
			}(chunkRow.ObjectKey)

			chunkRow.Status = model.ChunkUploadStatusFlagged
			h.db.Save(&chunkRow)
			uploadSession.Status = model.UploadStatusFlaggedMalicious
			h.db.Save(&uploadSession)

			wrappedErr := fmt.Errorf("etag mismatch (upload_id: %s, chunk_idx: %d): %w", uploadRequest.UploadID, uploadRequest.ChunkIndex, err)
			return apperrors.NewInternal(wrappedErr)
		}

		if size != chunkRow.DeclaredSize {
			uploadLogger.WarnContext(r.Context(), "size of chunk declared by client does not match with verified size", "object_key", chunkRow.ObjectKey, "declared_size", chunkRow.DeclaredSize, "true_size", size)
		}

		chunkRow.Etag = etag
		chunkRow.VerifiedSize = size
		chunkRow.VerifiedAt = time.Now()
		chunkRow.Status = model.ChunkUploadStatusVerified

		result := h.db.Save(&chunkRow)
		if result.Error != nil {
			wrappedErr := fmt.Errorf("chunk etag, size, timestamp, status could not be saved (upload_id: %s, chunk_idx: %d, size: %d): %w", uploadRequest.UploadID, uploadRequest.ChunkIndex, size, result.Error)
			return apperrors.NewInternal(wrappedErr)
		}
	}

	var verifiedCount int64
	h.db.Model(&model.UploadChunk{}).
		Where("upload_id = ? AND status = ?", uploadRequest.UploadID, model.ChunkUploadStatusVerified).
		Count(&verifiedCount)

	if verifiedCount != uploadSession.TotalChunks {
		w.WriteHeader(http.StatusOK)
		return nil
	}

	log.Print("upload session complete, all chunks verified, will mark upload as complete")
	uploadSession.Status = model.UploadStatusComplete
	if err := h.db.Save(&uploadSession).Error; err != nil {
		log.Printf("failed to mark upload session complete: %v", err)
		// don't fail the whole request over this — the chunk itself verified successfully;
		// completion can be caught by a reconciliation sweep if this save fails
	}
	log.Print("creating cryptographic hierarchy (link,node,file_block)")
	log.Printf("will clean upload_chunks and upload session")

	linkUUID := uuid.New()

	// TODO: Extract actual OwnerID from your DPoP/JWT auth claims or session
	// For testing/MVP if not extracted yet, fetch the parent node's owner:
	var parentNode model.Node
	if err := h.db.First(&parentNode, "id = ? AND owner_id = ?", uploadSession.ParentNodeID, uploadSession.UserID).Error; err != nil {
		return apperrors.NewNotFound("parent folder not found", nil)
	}
	ownerID := uploadSession.UserID

	// 1. Create the File Node
	fileNode := model.Node{
		ID:                uploadSession.NodeID,
		Type:              model.NodeTypeFile,
		EncryptedMetadata: uploadSession.EncryptedMetadata,
		MetadataNonce:     uploadSession.MetadataNonce,
		OwnerID:           ownerID,
		NodePublicKey:     uploadSession.NodePublicKey,
		WrappedNodeKey:    uploadSession.WrappedNodeKey,
		NodePrivNonce:     uploadSession.NodePrivNonce,
		Signature:         uploadSession.SignedEncryptedNodePassphrase,
	}

	// 2. Create the Link inside the parent folder
	fileLink := model.Link{
		ID:                            linkUUID,
		ParentNodeID:                  &parentNode.ID,
		ChildNodeID:                   &uploadSession.NodeID,
		EncryptedName:                 uploadSession.EncryptedName,
		NameNonce:                     uploadSession.NameNonce,
		EncryptedNodePassphrase:       uploadSession.EncryptedNodePassphrase,
		SignedEncryptedNodePassphrase: uploadSession.SignedEncryptedNodePassphrase,
		AuthorID:                      ownerID,
	}

	var verifiedChunks []model.UploadChunk
	if err := h.db.Where("upload_id = ? AND status = ?", uploadSession.ID, model.ChunkUploadStatusVerified).
		Order("chunk_index asc").Find(&verifiedChunks).Error; err != nil {
		log.Printf("failed to fetch verified chunks: %v", err)
		return apperrors.NewInternal(fmt.Errorf("internal server error"))
	}

	var trueTotalSize int64
	var blocks []model.FileBlock
	for _, chunk := range verifiedChunks {
		trueTotalSize += chunk.VerifiedSize
		blocks = append(blocks, model.FileBlock{
			ID:        uuid.New(),
			NodeID:    uploadSession.NodeID,
			Index:     int(chunk.ChunkIndex),
			Bucket:    "default",
			ObjectKey: chunk.ObjectKey,
			Size:      int(chunk.VerifiedSize),
		})
	}

	// Overwrite the Node's size with the VERIFIED physical size!
	fileNode.SizeBytes = trueTotalSize

	// 4. Save everything in an atomic Postgres transaction!
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&fileNode).Error; err != nil {
			return err
		}
		if err := tx.Create(&fileLink).Error; err != nil {
			return err
		}
		if len(blocks) > 0 {
			if err := tx.Create(&blocks).Error; err != nil {
				return err
			}
		}

		// Clean up the upload session and chunks since it's now completed successfully
		if err := tx.Unscoped().Where("upload_id = ?", uploadSession.ID).Delete(&model.UploadChunk{}).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Delete(&uploadSession).Error; err != nil {
			return err
		}

		// Increment the user's storage quota safely (atomic)
		if trueTotalSize > 0 {
			if err := tx.Model(&model.User{}).Where("id = ?", ownerID).UpdateColumn("storage_used", gorm.Expr("storage_used + ?", trueTotalSize)).Error; err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		log.Printf("Failed to finish file upload in DB: %v", err)
		return apperrors.NewInternal(fmt.Errorf("database error while saving file metadata"))
	}

	w.WriteHeader(http.StatusCreated)
	err = json.NewEncoder(w).Encode(map[string]string{"status": "created", "nodeId": uploadSession.NodeID.String()})
	if err != nil {
		return apperrors.NewInternal(err)
	}
	return nil
}

// func (h *Handler) FinishUpload(w http.ResponseWriter, r *http.Request) {
// 	if r.Method != http.MethodPost {
// 		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
// 		return nil
// 	}

// 	w.Header().Set("Content-Type", "application/json")

// 	var req dto.FinishFileUploadRequest
// 	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
// 		http.Error(w, err.Error(), http.StatusBadRequest)
// 		return nil
// 	}

// 	nodeUUID, err := uuid.Parse(req.NodeID)
// 	if err != nil {
// 		http.Error(w, "invalid nodeId uuid", http.StatusBadRequest)
// 		return nil
// 	}

// 	parentUUID, err := uuid.Parse(req.ParentNodeID)
// 	if err != nil {
// 		http.Error(w, "invalid parentNodeId uuid", http.StatusBadRequest)
// 		return nil
// 	}

// 	linkUUID := uuid.New()

// 	// TODO: Extract actual OwnerID from your DPoP/JWT auth claims or session
// 	// For testing/MVP if not extracted yet, fetch the parent node's owner:
// 	var parentNode model.Node
// 	if err := h.db.First(&parentNode, "id = ?", parentUUID).Error; err != nil {
// 		http.Error(w, "parent folder not found", http.StatusNotFound)
// 		return nil
// 	}
// 	ownerID := parentNode.OwnerID

// 	// 1. Create the File Node
// 	fileNode := model.Node{
// 		ID:                nodeUUID,
// 		Type:              model.NodeTypeFile,
// 		SizeBytes:         req.SizeBytes,
// 		EncryptedMetadata: req.EncryptedMetadata,
// 		MetadataNonce:     req.MetadataNonce,
// 		OwnerID:           ownerID,
// 		NodePublicKey:     req.NodePublicKey,
// 		WrappedNodeKey:    req.WrappedNodeKey,
// 		NodePrivNonce:     req.NodePrivNonce,
// 		Signature:         req.SignedEncryptedNodePassphrase,
// 	}

// 	// 2. Create the Link inside the parent folder
// 	fileLink := model.Link{
// 		ID:                            linkUUID,
// 		ParentNodeID:                  &parentUUID,
// 		ChildNodeID:                   &nodeUUID,
// 		EncryptedName:                 req.EncryptedName,
// 		NameNonce:                     req.NameNonce,
// 		EncryptedNodePassphrase:       req.EncryptedNodePassphrase,
// 		SignedEncryptedNodePassphrase: req.SignedEncryptedNodePassphrase,
// 		AuthorID:                      ownerID,
// 	}

// 	// 3. Security Check: Concurrently fetch TRUE chunk sizes directly from S3
// 	var blocks []model.FileBlock
// 	var trueTotalSize int64
// 	var mu sync.Mutex

// 	var wg sync.WaitGroup
// 	errCh := make(chan error, req.TotalChunks)

// 	for i := 0; i < req.TotalChunks; i++ {
// 		wg.Add(1)
// 		go func(chunkIndex int) {
// 			defer wg.Done()

// 			nonce := ""
// 			if chunkIndex < len(req.ChunkNonces) {
// 				nonce = req.ChunkNonces[chunkIndex]
// 			}

// 			objectKey := fmt.Sprintf("%s/chunk_%d", req.NodeID, chunkIndex)

// 			// Verify physical size directly from S3!
// 			size, _, _, err := h.storage.GetChunkSize(r.Context(), objectKey)
// 			if err != nil {
// 				errCh <- fmt.Errorf("chunk %d missing in S3: %v", chunkIndex, err)
// 				return nil
// 			}

// 			mu.Lock()
// 			trueTotalSize += size
// 			blocks = append(blocks, model.FileBlock{
// 				ID:        uuid.New(),
// 				NodeID:    nodeUUID,
// 				Index:     chunkIndex,
// 				Bucket:    "default",
// 				ObjectKey: objectKey,
// 				Size:      int(size),
// 			})
// 			mu.Unlock()
// 		}(i)
// 	}

// 	wg.Wait()
// 	close(errCh)

// 	if len(errCh) > 0 {
// 		err := <-errCh
// 		http.Error(w, err.Error(), http.StatusBadRequest)
// 		return nil
// 	}

// 	// Overwrite the Node's size with the VERIFIED physical size!
// 	fileNode.SizeBytes = trueTotalSize

// 	// 4. Save everything in an atomic Postgres transaction!
// 	err = h.db.Transaction(func(tx *gorm.DB) error {
// 		if err := tx.Create(&fileNode).Error; err != nil {
// 			return err
// 		}
// 		if err := tx.Create(&fileLink).Error; err != nil {
// 			return err
// 		}
// 		if len(blocks) > 0 {
// 			if err := tx.Create(&blocks).Error; err != nil {
// 				return err
// 			}
// 		}
// 		return nil
// 	})

// 	if err != nil {
// 		log.Printf("Failed to finish file upload in DB: %v", err)
// 		http.Error(w, "database error while saving file metadata", http.StatusInternalServerError)
// 		return nil
// 	}

// 	w.WriteHeader(http.StatusCreated)
// 	json.NewEncoder(w).Encode(map[string]string{"status": "created", "nodeId": req.NodeID})
// }

func (h *Handler) Files(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodGet {
		return apperrors.NewMethodNotAllowed("method not allowed")
	}

	// TODO: For MVP, we will fetch the dummy parent folder we used during upload:
	// "229474bd-9982-4999-bbc7-9bcd8577791c"
	parentFolderID := r.URL.Query().Get("folderId")
	if parentFolderID == "" {
		// If no folderId is passed, fetch the root folder for this user!
		// For now, grab the first Folder node:
		var rootNode model.Node
		h.db.Where("type = ?", model.NodeTypeFolder).First(&rootNode)
		parentFolderID = rootNode.ID.String()
	}

	var links []model.Link
	// Preload the ChildNode so we get SizeBytes and Type
	// Preload Author and Author.KeyStore for signature verification
	err := h.db.Preload("ChildNode").
		Preload("Author").
		Preload("Author.KeyStore").
		Where("parent_node_id = ?", parentFolderID).
		Find(&links).Error
	if err != nil {
		return apperrors.NewInternal(fmt.Errorf("failed to fetch files"))
	}

	var response []dto.FileListResponseItem
	for _, link := range links {
		hasChildren := false
		if link.ChildNode.Type == model.NodeTypeFolder {
			var count int64
			h.db.Model(&model.Link{}).Where("parent_node_id = ?", link.ChildNodeID).Count(&count)
			if count > 0 {
				hasChildren = true
			}
		}

		response = append(response, dto.FileListResponseItem{
			NodeID:                        link.ChildNodeID.String(),
			Type:                          string(link.ChildNode.Type),
			SizeBytes:                     link.ChildNode.SizeBytes,
			EncryptedName:                 link.EncryptedName,
			NameNonce:                     link.NameNonce,
			EncryptedNodePassphrase:       link.EncryptedNodePassphrase,
			SignedEncryptedNodePassphrase: link.SignedEncryptedNodePassphrase,

			NodePublicKey:  link.ChildNode.NodePublicKey,
			WrappedNodeKey: link.ChildNode.WrappedNodeKey,
			NodePrivNonce:  link.ChildNode.NodePrivNonce,

			HasChildren: hasChildren,
			CreatedAt:   link.CreatedAt,

			EncryptedMetadata: link.ChildNode.EncryptedMetadata,
			MetadataNonce:     link.ChildNode.MetadataNonce,

			AuthorEmail:            link.Author.EncryptedEmail,
			AuthorSigningPublicKey: link.Author.KeyStore.AccountSigningPublicKey,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(response)
	if err != nil {
		return apperrors.NewInternal(err)
	}
	return nil
}

func (h *Handler) Download(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodGet {
		return apperrors.NewMethodNotAllowed("method not allowed")
	}

	nodeID := r.URL.Query().Get("nodeId")
	if nodeID == "" {
		return apperrors.NewBadRequest("missing nodeId", nil)
	}

	userIDStr, ok := r.Context().Value(contextkeys.UserIDKey).(uuid.UUID)
	if !ok {
		return apperrors.NewUnauthorized("unauthorized", nil)
	}
	userID, err := uuid.Parse(userIDStr.String())
	if err != nil {
		return apperrors.NewUnauthorized("unauthorized", err)
	}

	if err := h.db.Where("node_id = ? and owner_id = ?", nodeID, userID).First(&model.Node{}).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperrors.NewForbidden("insufficient permissions to reach this node", nil)
		}
		return apperrors.NewInternal(err)
	}

	var blocks []model.FileBlock
	if err := h.db.Where("node_id = ?", nodeID).Order("index asc").Find(&blocks).Error; err != nil {
		return apperrors.NewNotFound("file blocks not found", nil)
	}

	if len(blocks) == 0 {
		return apperrors.NewNotFound("no chunks found for this file", nil)
	}

	// 2. Extract Object Keys and Generate URLs
	var objectKeys []string
	for _, block := range blocks {
		objectKeys = append(objectKeys, block.ObjectKey)
	}

	urls, err := h.storage.GenerateDownloadUrls(r.Context(), objectKeys)
	if err != nil {
		return apperrors.NewInternal(fmt.Errorf("failed to generate download links"))
	}

	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(map[string]interface{}{
		"presignedUrls": urls,
	})
	if err != nil {
		return apperrors.NewInternal(err)
	}
	return nil
}

func (h *Handler) DownloadUrls(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodPost {
		return apperrors.NewMethodNotAllowed("method not allowed")
	}

	var req dto.DownloadUrlsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return apperrors.NewBadRequest("invalid JSON payload", err)
	}

	if req.NodeID == "" {
		return apperrors.NewBadRequest("missing nodeId", nil)
	}

	var useOldCode bool

	if len(req.ChunkIndices) == 0 {
		//TODO: for now if chunk indices are empty, we will just return all urls (using old code)
		//return apperrors.NewBadRequest("chunkIndices cannot be empty", nil)
		useOldCode = true
	} else {
		useOldCode = false
	}

	if useOldCode {

		userIDStr, ok := r.Context().Value(contextkeys.UserIDKey).(uuid.UUID)
		if !ok {
			return apperrors.NewUnauthorized(userIDStr.String(), nil)
		}
		userID, err := uuid.Parse(userIDStr.String())
		if err != nil {
			return apperrors.NewUnauthorized("unauthorized", err)
		}

		if err := h.db.Where("id = ? and owner_id = ?", req.NodeID, userID).First(&model.Node{}).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperrors.NewForbidden("insufficient permissions to reach this node", nil)
			}
			return apperrors.NewInternal(err)
		}

		var blocks []model.FileBlock
		if err := h.db.Where("node_id = ?", req.NodeID).Order("index asc").Find(&blocks).Error; err != nil {
			return apperrors.NewNotFound("file blocks not found", nil)
		}

		if len(blocks) == 0 {
			return apperrors.NewNotFound("no chunks found for this file", nil)
		}

		// 2. Extract Object Keys and Generate URLs
		var objectKeys []string
		for _, block := range blocks {
			objectKeys = append(objectKeys, block.ObjectKey)
		}

		urls, err := h.storage.GenerateDownloadUrls(r.Context(), objectKeys)
		if err != nil {
			return apperrors.NewInternal(fmt.Errorf("failed to generate download links"))
		}

		w.Header().Set("Content-Type", "application/json")
		err = json.NewEncoder(w).Encode(map[string]interface{}{
			"presignedUrls": urls,
		})
		if err != nil {
			return apperrors.NewInternal(err)
		}

		return nil
	}

	if len(req.ChunkIndices) > 10 {
		return apperrors.NewBadRequest("maximum 10 chunks allowed per request", nil)
	}

	userID, ok := r.Context().Value(contextkeys.UserIDKey).(uuid.UUID)
	if !ok {
		return apperrors.NewUnauthorized("unauthorized", nil)
	}

	// Rate limit: 4 requests per minute per (userID, nodeID)
	reqRateKey := fmt.Sprintf("rl:dl:req:1m:%s:%s", userID.String(), req.NodeID)
	reqCount, err := h.rdb.Incr(r.Context(), reqRateKey).Result()
	if err != nil {
		return apperrors.NewInternal(err)
	}
	if reqCount == 1 {
		h.rdb.Expire(r.Context(), reqRateKey, time.Minute)
	}
	if reqCount > 4 {
		return apperrors.NewRateLimited("too many download requests for this file. please slow down.")
	}

	if err := h.db.Where("id = ? and owner_id = ?", req.NodeID, userID).First(&model.Node{}).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperrors.NewForbidden("insufficient permissions to reach this node", nil)
		}
		return apperrors.NewInternal(err)
	}

	// Fetch requested blocks
	var blocks []model.FileBlock
	if err := h.db.Where("id = ? AND index IN ?", req.NodeID, req.ChunkIndices).Find(&blocks).Error; err != nil {
		return apperrors.NewInternal(err)
	}
	if len(blocks) == 0 {
		return apperrors.NewNotFound("no matching chunks found for this file", nil)
	}

	// Verify all requested indices were found
	foundIndices := make(map[int]bool)
	for _, b := range blocks {
		foundIndices[b.Index] = true
	}
	for _, idx := range req.ChunkIndices {
		if !foundIndices[idx] {
			return apperrors.NewBadRequest(fmt.Sprintf("chunk index %d not found", idx), nil)
		}
	}

	// Chunk-level anti-replay limit
	hash1h := fmt.Sprintf("dl:stats:1h:%s:%s", req.NodeID, userID.String())
	hash24h := fmt.Sprintf("dl:stats:24h:%s:%s", req.NodeID, userID.String())

	pipe := h.rdb.Pipeline()
	cmds1h := make(map[int]*redis.IntCmd)
	cmds24h := make(map[int]*redis.IntCmd)

	for _, idx := range req.ChunkIndices {
		field := fmt.Sprintf("chunk:%d", idx)
		cmds1h[idx] = pipe.HIncrBy(r.Context(), hash1h, field, 1)
		cmds24h[idx] = pipe.HIncrBy(r.Context(), hash24h, field, 1)
	}

	pipe.Expire(r.Context(), hash1h, time.Hour)
	pipe.Expire(r.Context(), hash24h, 24*time.Hour)

	_, err = pipe.Exec(r.Context())
	if err != nil {
		return apperrors.NewInternal(err)
	}

	for idx, cmd := range cmds1h {
		if count, err := cmd.Result(); err == nil && count > 3 {
			return apperrors.NewRateLimited(fmt.Sprintf("chunk %d requested too many times in the last hour", idx))
		}
	}
	for idx, cmd := range cmds24h {
		if count, err := cmd.Result(); err == nil && count > 9 {
			return apperrors.NewRateLimited(fmt.Sprintf("chunk %d requested too many times in the last 24 hours", idx))
		}
	}

	// 2. Extract Object Keys and Generate URLs
	var objectKeys []string
	for _, block := range blocks {
		objectKeys = append(objectKeys, block.ObjectKey)
	}

	urls, err := h.storage.GenerateDownloadUrls(r.Context(), objectKeys)
	if err != nil {
		return apperrors.NewInternal(fmt.Errorf("failed to generate download links"))
	}

	var responseChunks []dto.DownloadUrlResponseItem
	for i, block := range blocks {
		responseChunks = append(responseChunks, dto.DownloadUrlResponseItem{
			Index:     block.Index,
			URL:       urls[i],
			SizeBytes: int64(block.Size),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(map[string]interface{}{
		"chunks": responseChunks,
	})
	if err != nil {
		return apperrors.NewInternal(err)
	}
	return nil
}

func (h *Handler) GetRootFolder(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodGet {
		return apperrors.NewMethodNotAllowed("method not allowed")
	}

	userID, ok := r.Context().Value(contextkeys.UserIDKey).(uuid.UUID)
	if !ok {
		return apperrors.NewUnauthorized("unauthorized", nil)
	}

	var shareMember model.ShareMember
	err := h.db.Preload("Share").
		Joins("JOIN shares ON shares.id = share_members.share_id").
		Where("share_members.user_id = ? AND shares.type = ?", userID, model.ShareTypeDefault).
		First(&shareMember).Error
	if err != nil {
		return apperrors.NewNotFound("share member not found", nil)
	}

	var rootLink model.Link
	if err := h.db.Preload("ChildNode").Where("id = ?", shareMember.Share.TargetLinkID).First(&rootLink).Error; err != nil {
		return apperrors.NewNotFound("root link not found", nil)
	}

	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(map[string]string{
		"sharePublicKey":                   shareMember.Share.SharePublicKey,
		"wrappedSharePrivateKey":           shareMember.Share.WrappedSharePrivateKey,
		"sharePrivNonce":                   shareMember.Share.SharePrivNonce,
		"encryptedSharePassphraseForOwner": shareMember.EncryptedSharePassphrase, // Now it correctly comes from the Member table!

		"nodeId":                      rootLink.ChildNode.ID.String(),
		"nodePublicKey":               rootLink.ChildNode.NodePublicKey,
		"wrappedNodePrivateKey":       rootLink.ChildNode.WrappedNodeKey,
		"nodePrivNonce":               rootLink.ChildNode.NodePrivNonce,
		"encryptedRootNodePassphrase": rootLink.EncryptedNodePassphrase,
	})
	if err != nil {
		return apperrors.NewInternal(err)
	}
	return nil
}

func (h *Handler) CreateFolder(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodPost {
		return apperrors.NewMethodNotAllowed("method not allowed")
	}
	var req dto.CreateFolderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return apperrors.NewBadRequest("invalid request body", nil)
	}
	nodeUUID := uuid.New()
	linkUUID := uuid.New()
	// 1. Prepare the Folder Node
	storedNode := model.Node{
		ID:             nodeUUID,
		Type:           model.NodeTypeFolder,
		OwnerID:        req.Link.AuthorID, // The creator owns the node
		NodePublicKey:  req.Node.NodePublicKey,
		WrappedNodeKey: req.Node.WrappedNodeKey,
		NodePrivNonce:  req.Node.NodePrivNonce,
		Signature:      req.Node.Signature,
	}
	// 2. Prepare the Link connecting the Parent to this new Folder
	storedLink := model.Link{
		ID:                            linkUUID,
		ParentNodeID:                  &req.Link.ParentNodeID,
		ChildNodeID:                   &nodeUUID,
		EncryptedName:                 req.Link.EncryptedName,
		NameNonce:                     req.Link.NameNonce,
		EncryptedNodePassphrase:       req.Link.EncryptedNodePassphrase,
		SignedEncryptedNodePassphrase: req.Link.SignedEncryptedNodePassphrase,
		AuthorID:                      req.Link.AuthorID,
	}
	// 3. Save both safely in a Transaction
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&storedNode).Error; err != nil {
			return err
		}
		if err := tx.Create(&storedLink).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return apperrors.NewInternal(fmt.Errorf("failed to create folder"))
	}
	w.WriteHeader(http.StatusCreated)
	err = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "nodeId": nodeUUID.String()})
	if err != nil {
		return apperrors.NewInternal(err)
	}
	return nil
}

func (h *Handler) TrashFile(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodDelete {
		return apperrors.NewMethodNotAllowed("method not allowed")
	}

	nodeID := r.URL.Query().Get("nodeId")
	parentFolderID := r.URL.Query().Get("parentFolderId")

	if nodeID == "" || parentFolderID == "" {
		return apperrors.NewBadRequest("missing parameters", nil)
	}

	// Soft Deletes the Link, instantly hiding it from the folder!
	err := h.db.Where("child_node_id = ? AND parent_node_id = ?", nodeID, parentFolderID).Delete(&model.Link{}).Error
	if err != nil {
		return apperrors.NewInternal(fmt.Errorf("failed to trash file"))
	}

	w.WriteHeader(http.StatusOK)
	return nil
}

func (h *Handler) RestoreFile(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodPost {
		return apperrors.NewMethodNotAllowed("method not allowed")
	}

	nodeID := r.URL.Query().Get("nodeId")
	parentFolderID := r.URL.Query().Get("parentFolderId")

	// Restore the item itself AND all its trashed ancestor folders using a recursive CTE.
	// This prevents the item from being stranded inside a still-trashed parent folder.
	//
	// The CTE walks UP from the item's parent to the root, collecting every link
	// whose child_node_id appears in the ancestor chain. It then restores any of
	// those links that are soft-deleted (deleted_at IS NOT NULL).
	query := `
		WITH RECURSIVE ancestors AS (
			-- Base: the direct parent link of the item being restored
			SELECT l.child_node_id, l.parent_node_id
			FROM links l
			WHERE l.child_node_id = ? AND l.deleted_at IS NOT NULL

			UNION ALL

			-- Recursively walk up: find the parent's own link (if it too is trashed)
			SELECT l.child_node_id, l.parent_node_id
			FROM links l
			INNER JOIN ancestors a ON l.child_node_id = a.parent_node_id
			WHERE l.deleted_at IS NOT NULL
		)
		UPDATE links SET deleted_at = NULL
		WHERE child_node_id IN (SELECT child_node_id FROM ancestors)
		   OR (child_node_id = ? AND parent_node_id = ?)
	`

	err := h.db.Exec(query, parentFolderID, nodeID, parentFolderID).Error
	if err != nil {
		log.Printf("failed to restore file %s: %v", nodeID, err)
		return apperrors.NewInternal(fmt.Errorf("failed to restore file"))
	}

	w.WriteHeader(http.StatusOK)
	return nil
}

func (h *Handler) RenameFile(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodPatch {
		return apperrors.NewMethodNotAllowed("method not allowed")
	}

	nodeID := r.URL.Query().Get("nodeId")
	parentFolderID := r.URL.Query().Get("parentFolderId")

	var req dto.RenameRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return apperrors.NewBadRequest("invalid body", nil)
	}

	err := h.db.Model(&model.Link{}).
		Where("child_node_id = ? AND parent_node_id = ?", nodeID, parentFolderID).
		Updates(map[string]interface{}{
			"encrypted_name": req.EncryptedName,
			"name_nonce":     req.NameNonce,
		}).Error

	if err != nil {
		return apperrors.NewInternal(fmt.Errorf("failed to update link"))
	}

	w.WriteHeader(http.StatusOK)
	return nil
}

func (h *Handler) ListTrash(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodGet {
		return apperrors.NewMethodNotAllowed("method not allowed")
	}

	userID, ok := r.Context().Value(contextkeys.UserIDKey).(uuid.UUID)
	if !ok {
		return apperrors.NewUnauthorized("unauthorized", nil)
	}

	var links []model.Link
	err := h.db.Unscoped().
		Joins("JOIN nodes ON nodes.id = links.child_node_id").
		Where("nodes.owner_id = ?", userID).
		Where("links.deleted_at IS NOT NULL").
		Preload("ChildNode").
		Find(&links).Error

	if err != nil {
		return apperrors.NewInternal(fmt.Errorf("failed to fetch trash"))
	}

	var response []dto.FileListResponseItem
	for _, link := range links {
		parentNodeID := ""
		if link.ParentNodeID != nil {
			parentNodeID = link.ParentNodeID.String()
		}
		response = append(response, dto.FileListResponseItem{
			NodeID:                        link.ChildNodeID.String(),
			ParentNodeID:                  parentNodeID,
			Type:                          string(link.ChildNode.Type),
			EncryptedName:                 link.EncryptedName,
			NameNonce:                     link.NameNonce,
			EncryptedNodePassphrase:       link.EncryptedNodePassphrase,
			SignedEncryptedNodePassphrase: link.SignedEncryptedNodePassphrase,
			CreatedAt:                     link.CreatedAt,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(response)
	if err != nil {
		return apperrors.NewInternal(err)
	}
	return nil
}

func (h *Handler) getDescendantNodeIDs(parentFolderID string) ([]string, error) {
	var nodeIDs []string
	query := `
		WITH RECURSIVE Descendants AS (
			SELECT child_node_id FROM links WHERE parent_node_id = ?
			UNION
			SELECT l.child_node_id FROM links l
			INNER JOIN Descendants d ON l.parent_node_id = d.child_node_id
		)
		SELECT child_node_id FROM Descendants;
	`
	if err := h.db.Raw(query, parentFolderID).Scan(&nodeIDs).Error; err != nil {
		return nil, fmt.Errorf("failed to query descendants for folder %s: %w", parentFolderID, err)
	}
	return nodeIDs, nil
}

func (h *Handler) EmptyTrash(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodDelete {
		return apperrors.NewMethodNotAllowed("method not allowed")
	}

	// 1. Get the Authenticated User ID
	userID, ok := r.Context().Value(contextkeys.UserIDKey).(uuid.UUID)
	if !ok {
		return apperrors.NewUnauthorized("unauthorized", nil)
	}

	var trashedLinks []model.Link
	// Preload ChildNode so we know if it's a FOLDER or a FILE
	// 2. Scope the query to ONLY this user's files by joining the nodes table!
	err := h.db.Unscoped().
		Joins("JOIN nodes ON nodes.id = links.child_node_id").
		Where("nodes.owner_id = ?", userID).
		Where("links.deleted_at IS NOT NULL").
		Preload("ChildNode").
		Find(&trashedLinks).Error

	if err != nil {
		return apperrors.NewInternal(fmt.Errorf("failed to query trash"))
	}

	if len(trashedLinks) == 0 {
		w.WriteHeader(http.StatusOK)
		return nil
	}

	go h.wipeTrashedLinks(context.Background(), trashedLinks)

	w.WriteHeader(http.StatusAccepted)
	return nil
}

func (h *Handler) GetQuota(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodGet {
		return apperrors.NewMethodNotAllowed("method not allowed")
	}

	userID, ok := r.Context().Value(contextkeys.UserIDKey).(uuid.UUID)
	if !ok {
		return apperrors.NewUnauthorized("unauthorized", nil)
	}

	var user model.User
	if err := h.db.First(&user, "id = ?", userID).Error; err != nil {
		return apperrors.NewNotFound("user not found", nil)
	}

	w.Header().Set("Content-Type", "application/json")
	err := json.NewEncoder(w).Encode(map[string]interface{}{
		"usedBytes": user.StorageUsed,
		"maxBytes":  user.StorageQuota,
	})
	if err != nil {
		return apperrors.NewInternal(err)
	}
	return nil
}

func (h *Handler) MoveFile(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodPatch {
		return apperrors.NewMethodNotAllowed("method not allowed")
	}

	var req dto.MoveFileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return apperrors.NewBadRequest("invalid request", nil)
	}

	// 1. Validate UUIDs
	nodeUUID, err1 := uuid.Parse(req.NodeID)
	oldParentUUID, err2 := uuid.Parse(req.OldParentFolderID)
	newParentUUID, err3 := uuid.Parse(req.NewParentFolderID)
	if err1 != nil || err2 != nil || err3 != nil {
		return apperrors.NewBadRequest("invalid UUIDs", nil)
	}

	// 2. Find the exact Link connecting the File to the Old Folder
	var link model.Link
	if err := h.db.Where("child_node_id = ? AND parent_node_id = ?", nodeUUID, oldParentUUID).First(&link).Error; err != nil {
		return apperrors.NewNotFound("file not found in the specified source folder", nil)
	}

	// 3. Cryptographic Re-link! Update the parent and overwrite all crypto fields
	link.ParentNodeID = &newParentUUID
	link.EncryptedName = req.NewEncryptedName
	link.NameNonce = req.NewNameNonce
	link.EncryptedNodePassphrase = req.NewEncryptedNodePassphrase
	link.SignedEncryptedNodePassphrase = req.NewSignedEncryptedPassphrase

	// 4. Save the new Link to the database
	if err := h.db.Save(&link).Error; err != nil {
		return apperrors.NewInternal(fmt.Errorf("failed to move file"))
	}

	w.WriteHeader(http.StatusOK)
	return nil
}

func (h *Handler) GetAllFiles(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodGet {
		return apperrors.NewMethodNotAllowed("method not allowed")
	}

	// 1. Get the current User ID from the Auth Middleware Context!
	userID, ok := r.Context().Value(contextkeys.UserIDKey).(uuid.UUID)
	if !ok {
		return apperrors.NewUnauthorized("unauthorized", nil)
	}

	var links []model.Link
	// 2. Fetch every single file/folder the user owns in one highly optimized query
	// We join the `nodes` table so we can filter by the OwnerID!
	err := h.db.Joins("JOIN nodes ON nodes.id = links.child_node_id").
		Where("nodes.owner_id = ?", userID).
		Preload("ChildNode").
		Preload("Author").
		Preload("Author.KeyStore").
		Find(&links).Error

	if err != nil {
		return apperrors.NewInternal(fmt.Errorf("failed to fetch files for search index"))
	}

	var response []dto.FileListResponseItem
	for _, link := range links {
		// 1. Safely handle nil pointers for ParentNodeID and ChildNodeID
		parentIDStr := ""
		if link.ParentNodeID != nil {
			parentIDStr = link.ParentNodeID.String()
		}

		childIDStr := ""
		if link.ChildNodeID != nil {
			childIDStr = link.ChildNodeID.String()
		}

		response = append(response, dto.FileListResponseItem{
			NodeID:                        childIDStr,
			ParentNodeID:                  parentIDStr, // <-- Now safe from panics!
			Type:                          string(link.ChildNode.Type),
			SizeBytes:                     link.ChildNode.SizeBytes,
			EncryptedName:                 link.EncryptedName,
			NameNonce:                     link.NameNonce,
			EncryptedNodePassphrase:       link.EncryptedNodePassphrase,
			SignedEncryptedNodePassphrase: link.SignedEncryptedNodePassphrase,

			NodePublicKey:  link.ChildNode.NodePublicKey,
			WrappedNodeKey: link.ChildNode.WrappedNodeKey,
			NodePrivNonce:  link.ChildNode.NodePrivNonce,

			CreatedAt:         link.CreatedAt,
			EncryptedMetadata: link.ChildNode.EncryptedMetadata,
			MetadataNonce:     link.ChildNode.MetadataNonce,

			AuthorEmail:            link.Author.EncryptedEmail,
			AuthorSigningPublicKey: link.Author.KeyStore.AccountSigningPublicKey,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(response)
	if err != nil {
		return apperrors.NewInternal(err)
	}
	return nil
}

func (h *Handler) GetFilePath(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodGet {
		return apperrors.NewMethodNotAllowed("method not allowed")
	}

	nodeID := r.URL.Query().Get("nodeId")
	if nodeID == "" {
		return apperrors.NewBadRequest("missing nodeId", nil)
	}

	// Verify the user is authenticated (ensure they own the nodes later!)
	_, ok := r.Context().Value(contextkeys.UserIDKey).(uuid.UUID)
	if !ok {
		return apperrors.NewUnauthorized("unauthorized", nil)
	}

	var response []dto.FileListResponseItem

	// 1. PostgreSQL Recursive CTE
	// This traces the path from the requested Node UP to the Root Folder!
	query := `
		WITH RECURSIVE path_tree AS (
			-- Base case: The requested node
			SELECT
				l.parent_node_id, l.child_node_id, l.encrypted_name, l.name_nonce,
				l.encrypted_node_passphrase, l.signed_encrypted_node_passphrase, l.created_at,
				1 as depth
			FROM links l
			WHERE l.child_node_id = ?

			UNION ALL

			-- Recursive step: Join the parent folder's link
			SELECT
				parent.parent_node_id, parent.child_node_id, parent.encrypted_name, parent.name_nonce,
				parent.encrypted_node_passphrase, parent.signed_encrypted_node_passphrase, parent.created_at,
				pt.depth + 1
			FROM links parent
			INNER JOIN path_tree pt ON pt.parent_node_id = parent.child_node_id
		)
		-- Finally, join the nodes table so we can get the Public/Wrapped Keys and Type
		SELECT
			pt.child_node_id as node_id,
			COALESCE(pt.parent_node_id::text, '') as parent_node_id,
			n.type,
			n.size_bytes,
			pt.encrypted_name,
			pt.name_nonce,
			pt.encrypted_node_passphrase,
			pt.signed_encrypted_node_passphrase,
			n.node_public_key,
			n.wrapped_node_key,
			n.node_priv_nonce,
			pt.created_at,
			n.encrypted_metadata,
			n.metadata_nonce
		FROM path_tree pt
		JOIN nodes n ON n.id = pt.child_node_id
		ORDER BY pt.depth DESC; -- Reverse the order so Root is first!
	`

	err := h.db.Raw(query, nodeID).Scan(&response).Error
	if err != nil {
		return apperrors.NewInternal(fmt.Errorf("failed to resolve cryptographic path"))
	}

	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(response)
	if err != nil {
		return apperrors.NewInternal(err)
	}
	return nil
}

func (h *Handler) ShareFolder(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodPost {
		return apperrors.NewMethodNotAllowed("method not allowed")
	}

	authorID, ok := r.Context().Value(contextkeys.UserIDKey).(uuid.UUID)
	if !ok {
		return apperrors.NewUnauthorized("unauthorized", nil)
	}

	var req struct {
		TargetNodeID    string `json:"targetNodeId"`
		RecipientUserID string `json:"recipientUserId"`

		// Share Keys
		SharePublicKey         string `json:"sharePublicKey"`
		WrappedSharePrivateKey string `json:"wrappedSharePrivateKey"`
		SharePrivNonce         string `json:"sharePrivNonce"`

		// Sealed Passphrase for Recipient
		EncryptedSharePassphraseForOwner string `json:"encryptedSharePassphraseForOwner"`
		SignedEncryptedSharePassphrase   string `json:"signedEncryptedSharePassphrase"`

		// New Root Link Data
		EncryptedTargetNodePassphrase string `json:"encryptedTargetNodePassphrase"`
		EncryptedName                 string `json:"encryptedName"`
		NameNonce                     string `json:"nameNonce"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return apperrors.NewBadRequest("bad request", nil)
	}

	targetNodeUUID := uuid.MustParse(req.TargetNodeID)
	recipientUUID := uuid.MustParse(req.RecipientUserID)

	err := h.db.Transaction(func(tx *gorm.DB) error {
		// 1. Create a NEW Link that acts as a "Root" (ParentNodeID = null) for the recipient
		newLink := model.Link{
			ID:                      uuid.New(),
			ParentNodeID:            nil, // IT IS A ROOT NOW!
			ChildNodeID:             &targetNodeUUID,
			EncryptedName:           req.EncryptedName,
			NameNonce:               req.NameNonce,
			EncryptedNodePassphrase: req.EncryptedTargetNodePassphrase,
			AuthorID:                authorID, // John created the link
		}
		if err := tx.Create(&newLink).Error; err != nil {
			return err
		}

		// 2. Create the Share object holding the new Share Keys
		newShare := model.Share{
			ID:                     uuid.New(),
			TargetLinkID:           newLink.ID,
			Type:                   model.ShareTypeShared,
			OwnerID:                recipientUUID, // Jane is the owner of this share perspective
			SharePublicKey:         req.SharePublicKey,
			WrappedSharePrivateKey: req.WrappedSharePrivateKey,
			SharePrivNonce:         req.SharePrivNonce,
		}
		if err := tx.Create(&newShare).Error; err != nil {
			return err
		}

		// 3. Bind Jane to the Share and give her the sealed passphrase!
		shareMember := model.ShareMember{
			ShareID:                        newShare.ID,
			UserID:                         recipientUUID,
			Permissions:                    3, // Read/Write
			EncryptedSharePassphrase:       req.EncryptedSharePassphraseForOwner,
			SignedEncryptedSharePassphrase: req.SignedEncryptedSharePassphrase,
		}
		if err := tx.Create(&shareMember).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return apperrors.NewInternal(fmt.Errorf("failed to execute share transaction"))
	}

	w.WriteHeader(http.StatusCreated)
	return nil
}

func (h *Handler) GetSharedFolders(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodGet {
		return apperrors.NewMethodNotAllowed("method not allowed")
	}

	userID, ok := r.Context().Value(contextkeys.UserIDKey).(uuid.UUID)
	if !ok {
		return apperrors.NewUnauthorized("unauthorized", nil)
	}

	// 1. Fetch all SHARED ShareMembers for this user
	var shareMembers []model.ShareMember
	err := h.db.Preload("Share").
		Joins("JOIN shares ON shares.id = share_members.share_id").
		Where("share_members.user_id = ? AND shares.type = ?", userID, model.ShareTypeShared).
		Find(&shareMembers).Error

	if err != nil {
		return apperrors.NewInternal(fmt.Errorf("failed to query shares"))
	}

	var response []map[string]string

	// 2. Loop through them and grab the TargetLink for each one
	for _, member := range shareMembers {
		var rootLink model.Link
		if err := h.db.Preload("ChildNode").Where("id = ?", member.Share.TargetLinkID).First(&rootLink).Error; err != nil {
			continue // Skip if corrupted
		}

		response = append(response, map[string]string{
			"sharePublicKey":                   member.Share.SharePublicKey,
			"wrappedSharePrivateKey":           member.Share.WrappedSharePrivateKey,
			"sharePrivNonce":                   member.Share.SharePrivNonce,
			"encryptedSharePassphraseForOwner": member.EncryptedSharePassphrase,

			"nodeId":                      rootLink.ChildNode.ID.String(),
			"nodePublicKey":               rootLink.ChildNode.NodePublicKey,
			"wrappedNodePrivateKey":       rootLink.ChildNode.WrappedNodeKey,
			"nodePrivNonce":               rootLink.ChildNode.NodePrivNonce,
			"encryptedRootNodePassphrase": rootLink.EncryptedNodePassphrase,

			// We also need the encrypted name so we can render it in the UI!
			"encryptedName": rootLink.EncryptedName,
			"nameNonce":     rootLink.NameNonce,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(response)
	if err != nil {
		return apperrors.NewInternal(err)
	}
	return nil
}
