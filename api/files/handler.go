package files

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"quartz/config"
	"quartz/internal/dto"
	"quartz/internal/middleware"
	"quartz/internal/model"
	"quartz/pkg/storage"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Handler struct {
	db      *gorm.DB
	storage storage.StorageService
}

func NewHandler(db *gorm.DB, storage storage.StorageService) *Handler {
	return &Handler{db: db, storage: storage}
}

// inits upload session and sends back upload & node id
func (h *Handler) InitUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	var uploadRequest dto.InitFileUploadRequest
	if err := json.NewDecoder(r.Body).Decode(&uploadRequest); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	userID := r.Context().Value(middleware.UserIDKey)
	if userID == nil {
		http.Error(w, "access token invalid", http.StatusUnauthorized)
		return
	}

	if uploadRequest.TotalChunks <= 0 || uploadRequest.TotalFileSize <= 0 {
		http.Error(w, "total chunks or total file size invalid", http.StatusUnprocessableEntity)
		return
	}

	const maxChunkSize = 4 * 1024 * 1024 //4MB

	parentUUID, err := uuid.Parse(uploadRequest.ParentNodeID)
	if err != nil {
		http.Error(w, "invalid parentNodeId uuid", http.StatusBadRequest)
		return
	}

	var parentNode model.Node
	if err := h.db.First(&parentNode, "id = ? AND owner_id = ?", parentUUID, userID).Error; err != nil {
		http.Error(w, "parent folder not found", http.StatusNotFound)
		return
	}

	var user model.User
	if err := h.db.First(&user, "id = ?", userID).Error; err != nil {
		http.Error(w, "user not found", http.StatusUnauthorized)
		return
	}

	if user.StorageUsed+uploadRequest.TotalFileSize > user.StorageQuota {
		http.Error(w, "quota exceeded", http.StatusForbidden)
		return
	}

	minPlausibleChunks := int(math.Ceil(float64(uploadRequest.TotalFileSize) / float64(maxChunkSize)))
	if uploadRequest.TotalChunks < int64(minPlausibleChunks) {
		http.Error(w, "totalChunks too low for declared file size", http.StatusUnprocessableEntity)
		return
	}

	const chunkCountSlack = 1.05 // 5% slack for per-chunk overhead
	maxPlausibleChunks := int(math.Ceil(float64(minPlausibleChunks)*chunkCountSlack)) + 1
	if uploadRequest.TotalChunks > int64(maxPlausibleChunks) {
		http.Error(w, "totalChunks too high for declared file size", http.StatusUnprocessableEntity)
		return
	}

	nodeID, nErr := uuid.NewV7()
	uploadID, uErr := uuid.NewV7()
	if nErr != nil || uErr != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	expiresAt := time.Now().UTC().Add(time.Duration(config.Cfg.Sweeper.UploadSessionExpiresHours) * time.Hour)

	chunkRows := []model.UploadChunk{}

	for i := int64(0); i < uploadRequest.TotalChunks; i++ {
		id, err := uuid.NewV7()
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		chunkRows = append(chunkRows, model.UploadChunk{
			ID:         id,
			UploadID:   uploadID,
			ChunkIndex: i,
			ObjectKey:  fmt.Sprintf("authors/%s/uploads/%s/nodes/%s/chunk_%d", user.ID.String(), uploadID.String(), nodeID.String(), i),
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

		log.Printf("chunkRows: %d", len(chunkRows))

		if err := tx.CreateInBatches(chunkRows, len(chunkRows)).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		http.Error(w, "could not init an upload session", http.StatusInternalServerError)
		return
	}

	uploadResponse := InitFileUploadResponse{
		UploadID:  uploadID,
		NodeID:    nodeID,
		ExpiresAt: expiresAt,
	}

	err = json.NewEncoder(w).Encode(uploadResponse)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
}

// validates chunk upload request and presigns an url for that chunk
func (h *Handler) Upload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	var uploadRequest dto.RequestChunkUploadRequest
	if err := json.NewDecoder(r.Body).Decode(&uploadRequest); err != nil {
		log.Printf("failed to decode RequestChunkUploadRequest")
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	userID := r.Context().Value(middleware.UserIDKey)
	if userID == nil {
		log.Printf("access token invalid")
		http.Error(w, "access token invalid", http.StatusUnauthorized)
		return
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
		log.Printf("user not found")
		http.Error(w, "user not found", http.StatusUnauthorized)
		return
	}

	var uploadSession model.Upload
	if err := h.db.First(&uploadSession, "id = ?", uploadRequest.UploadID).Error; err != nil {
		log.Printf("upload session not found")
		http.Error(w, "upload session not found", http.StatusForbidden)
		return
	}

	if !time.Now().Before(uploadSession.ExpiresAt) {
		log.Printf("upload session expired")
		http.Error(w, "upload session not found", http.StatusForbidden)
		return
	}

	if uploadSession.Status != model.UploadStatusPending {
		log.Printf("upload session is not pending (status: %s)", uploadSession.Status)
		http.Error(w, "upload session is no longer active", http.StatusForbidden)
		return
	}

	if user.ID != uploadSession.UserID {
		log.Printf("user is not the owner of the upload session")
		http.Error(w, "upload session not found", http.StatusForbidden)
		return
	}

	if uploadRequest.DeclaredSize <= 0 || uploadRequest.DeclaredSize > maxChunkSize {
		log.Printf("declared chunk size out of allowed range")
		http.Error(w, "declared chunk size out of allowed range", http.StatusUnprocessableEntity)
		return
	}

	if int64(uploadRequest.ChunkIndex) >= uploadSession.TotalChunks || uploadRequest.ChunkIndex < 0 {
		log.Printf("requested chunk index out of allowed range")
		http.Error(w, "requested chunk index out of allowed range", http.StatusForbidden)
		return
	}

	if user.StorageUsed+uploadRequest.DeclaredSize > user.StorageQuota {
		http.Error(w, "quota exceeded", http.StatusForbidden)
		return
	}

	var chunkRow model.UploadChunk
	if err := h.db.First(&chunkRow, "upload_id = ? AND chunk_index = ?", uploadRequest.UploadID, uploadRequest.ChunkIndex).Error; err != nil {
		log.Printf("failed to find chunk row %s", err)
		http.Error(w, "requested chunk not found", http.StatusForbidden)
		return
	}

	if chunkRow.Status != model.ChunkUploadStatusPending {
		log.Printf("error: requested chunk %d status is %s", chunkRow.ChunkIndex, chunkRow.Status)
		http.Error(w, "requested chunk not found", http.StatusForbidden)
		return
	}

	url, err := h.storage.GenerateUploadUrl(r.Context(), chunkRow.ObjectKey, expiry, uploadRequest.DeclaredSize, uploadRequest.ChunkHash)
	if err != nil {
		log.Printf("%w", err)
		http.Error(w, "failed to generate presigned url", http.StatusInternalServerError)
		return
	}

	chunkRow.GeneratedUrls++
	chunkRow.DeclaredSize = uploadRequest.DeclaredSize

	result := h.db.Save(&chunkRow)
	if result.Error != nil {
		log.Printf("chunk row generated urls amount and declared size could not be saved", err)
		http.Error(w, "failed to generate presigned url", http.StatusInternalServerError)
		return
	}

	uploadResponse := dto.RequestChunkUploadResponse{
		URL: url,
	}

	err = json.NewEncoder(w).Encode(uploadResponse)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
}

func (h *Handler) ReportChunkUploadDone(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	var uploadRequest dto.FinishChunkUploadRequest
	if err := json.NewDecoder(r.Body).Decode(&uploadRequest); err != nil {
		log.Printf("failed to decode FinishChunkUploadRequest")
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	userID := r.Context().Value(middleware.UserIDKey)
	if userID == nil {
		log.Printf("access token invalid")
		http.Error(w, "access token invalid", http.StatusUnauthorized)
		return
	}

	const maxChunkSize = 4*1024*1024 + 64 // 4MiB + small margin for AEAD overhead/headers

	var user model.User
	if err := h.db.First(&user, "id = ?", userID).Error; err != nil {
		log.Printf("user not found")
		http.Error(w, "user not found", http.StatusUnauthorized)
		return
	}

	var uploadSession model.Upload
	if err := h.db.First(&uploadSession, "id = ?", uploadRequest.UploadID).Error; err != nil {
		log.Printf("upload session not found")
		http.Error(w, "upload session not found", http.StatusForbidden)
		return
	}

	if !time.Now().Before(uploadSession.ExpiresAt) {
		log.Printf("upload session expired")
		http.Error(w, "upload session not found", http.StatusForbidden)
		return
	}

	if user.ID != uploadSession.UserID {
		log.Printf("user is not the owner of the upload session")
		http.Error(w, "upload session not found", http.StatusForbidden)
		return
	}

	if int64(uploadRequest.ChunkIndex) >= uploadSession.TotalChunks || uploadRequest.ChunkIndex < 0 {
		log.Printf("requested chunk index out of allowed range")
		http.Error(w, "requested chunk index out of allowed range", http.StatusForbidden)
		return
	}

	var chunkRow model.UploadChunk
	if err := h.db.First(&chunkRow, "upload_id = ? AND chunk_index = ?", uploadRequest.UploadID, uploadRequest.ChunkIndex).Error; err != nil {
		log.Printf("failed to find chunk row %s", err)
		http.Error(w, "requested chunk not found", http.StatusForbidden)
		return
	}

	if chunkRow.Status != model.ChunkUploadStatusPending {
		log.Printf("error: requested chunk %d status is %s", chunkRow.ChunkIndex, chunkRow.Status)
		http.Error(w, "requested chunk not found", http.StatusNotFound)
		return
	}

	size, etag, sha256, err := h.storage.GetChunkSize(r.Context(), chunkRow.ObjectKey)
	if err != nil {
		log.Printf("failed to get chunk size, etag and checksum: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	log.Printf("checksum is %s", sha256)

	if subtle.ConstantTimeCompare([]byte(sha256), []byte(uploadRequest.ChunkHash)) == 0 {
		log.Printf("integrity check failed", err)
		go func(objectKey string) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if delErr := h.storage.DeleteChunk(ctx, objectKey); delErr != nil {
				log.Printf("failed to delete (integrity check failed) chunk %s: %v", objectKey, delErr)
			}
		}(chunkRow.ObjectKey)
		http.Error(w, "integrity check failed", http.StatusForbidden)
		return
	}

	if size > maxChunkSize {
		log.Printf("CHUNK %s OVERSIZED: %d > %d", chunkRow.ObjectKey, size, maxChunkSize)
		go func(objectKey string) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if delErr := h.storage.DeleteChunk(ctx, objectKey); delErr != nil {
				log.Printf("failed to delete oversized chunk %s: %v", objectKey, delErr)
			}
		}(chunkRow.ObjectKey)

		chunkRow.Status = model.ChunkUploadStatusFlagged
		h.db.Save(&chunkRow)
		uploadSession.Status = model.UploadStatusFlaggedMalicious
		h.db.Save(&uploadSession)

		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	if etag != uploadRequest.Etag {
		log.Printf("CHUNK %s etags dont match: %s != %s", chunkRow.ObjectKey, etag, uploadRequest.Etag)

		go func(objectKey string) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if delErr := h.storage.DeleteChunk(ctx, objectKey); delErr != nil {
				log.Printf("failed to delete (mismatch etag) chunk %s: %v", objectKey, delErr)
			}
		}(chunkRow.ObjectKey)

		chunkRow.Status = model.ChunkUploadStatusFlagged
		h.db.Save(&chunkRow)
		uploadSession.Status = model.UploadStatusFlaggedMalicious
		h.db.Save(&uploadSession)

		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	if size != chunkRow.DeclaredSize {
		//log it and save verified size later
		log.Printf("log: size of chunk %s declared by client does not match with verified size: declared %d vs verified %d", chunkRow.ObjectKey, chunkRow.DeclaredSize, size)
	}

	chunkRow.Etag = etag
	chunkRow.VerifiedSize = size
	chunkRow.VerifiedAt = time.Now()
	chunkRow.Status = model.ChunkUploadStatusVerified

	result := h.db.Save(&chunkRow)
	if result.Error != nil {
		log.Printf("chunk etag, size, timestamp, status could not be saved: %v", result.Error)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	var verifiedCount int64
	h.db.Model(&model.UploadChunk{}).
		Where("upload_id = ? AND status = ?", uploadRequest.UploadID, model.ChunkUploadStatusVerified).
		Count(&verifiedCount)

	if verifiedCount != uploadSession.TotalChunks {
		w.WriteHeader(http.StatusOK)
		return
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
		http.Error(w, "parent folder not found", http.StatusNotFound)
		return
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
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
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
	err = h.db.Transaction(func(tx *gorm.DB) error {
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
		http.Error(w, "database error while saving file metadata", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"status": "created", "nodeId": uploadSession.NodeID.String()})
}

// func (h *Handler) FinishUpload(w http.ResponseWriter, r *http.Request) {
// 	if r.Method != http.MethodPost {
// 		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
// 		return
// 	}

// 	w.Header().Set("Content-Type", "application/json")

// 	var req dto.FinishFileUploadRequest
// 	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
// 		http.Error(w, err.Error(), http.StatusBadRequest)
// 		return
// 	}

// 	nodeUUID, err := uuid.Parse(req.NodeID)
// 	if err != nil {
// 		http.Error(w, "invalid nodeId uuid", http.StatusBadRequest)
// 		return
// 	}

// 	parentUUID, err := uuid.Parse(req.ParentNodeID)
// 	if err != nil {
// 		http.Error(w, "invalid parentNodeId uuid", http.StatusBadRequest)
// 		return
// 	}

// 	linkUUID := uuid.New()

// 	// TODO: Extract actual OwnerID from your DPoP/JWT auth claims or session
// 	// For testing/MVP if not extracted yet, fetch the parent node's owner:
// 	var parentNode model.Node
// 	if err := h.db.First(&parentNode, "id = ?", parentUUID).Error; err != nil {
// 		http.Error(w, "parent folder not found", http.StatusNotFound)
// 		return
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
// 				return
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
// 		return
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
// 		return
// 	}

// 	w.WriteHeader(http.StatusCreated)
// 	json.NewEncoder(w).Encode(map[string]string{"status": "created", "nodeId": req.NodeID})
// }

func (h *Handler) Files(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
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
	err := h.db.Preload("ChildNode").Where("parent_node_id = ?", parentFolderID).Find(&links).Error
	if err != nil {
		http.Error(w, "failed to fetch files", http.StatusInternalServerError)
		return
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
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (h *Handler) Download(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	nodeID := r.URL.Query().Get("nodeId")
	if nodeID == "" {
		http.Error(w, "missing nodeId", http.StatusBadRequest)
		return
	}

	// 1. Verify ownership/access here later. For MVP, we just fetch blocks:
	var blocks []model.FileBlock
	if err := h.db.Where("node_id = ?", nodeID).Order("index asc").Find(&blocks).Error; err != nil {
		http.Error(w, "file blocks not found", http.StatusNotFound)
		return
	}

	if len(blocks) == 0 {
		http.Error(w, "no chunks found for this file", http.StatusNotFound)
		return
	}

	// 2. Extract Object Keys and Generate URLs
	var objectKeys []string
	for _, block := range blocks {
		objectKeys = append(objectKeys, block.ObjectKey)
	}

	urls, err := h.storage.GenerateDownloadUrls(r.Context(), objectKeys)
	if err != nil {
		http.Error(w, "failed to generate download links", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"presignedUrls": urls,
	})
}

func (h *Handler) GetRootFolder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var shareMember model.ShareMember
	err := h.db.Preload("Share").
		Joins("JOIN shares ON shares.id = share_members.share_id").
		Where("share_members.user_id = ? AND shares.type = ?", userID, model.ShareTypeDefault).
		First(&shareMember).Error
	if err != nil {
		http.Error(w, "share member not found", http.StatusNotFound)
		return
	}

	var rootLink model.Link
	if err := h.db.Preload("ChildNode").Where("id = ?", shareMember.Share.TargetLinkID).First(&rootLink).Error; err != nil {
		http.Error(w, "root link not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
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
}

func (h *Handler) CreateFolder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req dto.CreateFolderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
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
		http.Error(w, "failed to create folder", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "nodeId": nodeUUID.String()})
}

func (h *Handler) TrashFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	nodeID := r.URL.Query().Get("nodeId")
	parentFolderID := r.URL.Query().Get("parentFolderId")

	if nodeID == "" || parentFolderID == "" {
		http.Error(w, "missing parameters", http.StatusBadRequest)
		return
	}

	// Soft Deletes the Link, instantly hiding it from the folder!
	err := h.db.Where("child_node_id = ? AND parent_node_id = ?", nodeID, parentFolderID).Delete(&model.Link{}).Error
	if err != nil {
		http.Error(w, "failed to trash file", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *Handler) RestoreFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	nodeID := r.URL.Query().Get("nodeId")
	parentFolderID := r.URL.Query().Get("parentFolderId")

	// Unscoped allows us to find the Trashed item, and setting deleted_at to NULL restores it!
	err := h.db.Unscoped().Model(&model.Link{}).
		Where("child_node_id = ? AND parent_node_id = ?", nodeID, parentFolderID).
		Update("deleted_at", nil).Error

	if err != nil {
		http.Error(w, "failed to restore file", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *Handler) RenameFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	nodeID := r.URL.Query().Get("nodeId")
	parentFolderID := r.URL.Query().Get("parentFolderId")

	var req dto.RenameRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	err := h.db.Model(&model.Link{}).
		Where("child_node_id = ? AND parent_node_id = ?", nodeID, parentFolderID).
		Updates(map[string]interface{}{
			"encrypted_name": req.EncryptedName,
			"name_nonce":     req.NameNonce,
		}).Error

	if err != nil {
		http.Error(w, "failed to update link", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *Handler) ListTrash(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	parentFolderID := r.URL.Query().Get("folderId")
	if parentFolderID == "" {
		http.Error(w, "missing folderId", http.StatusBadRequest)
		return
	}

	var links []model.Link
	err := h.db.Unscoped().Preload("ChildNode").
		Where("parent_node_id = ? AND deleted_at IS NOT NULL", parentFolderID).
		Find(&links).Error

	if err != nil {
		http.Error(w, "failed to fetch trash", http.StatusInternalServerError)
		return
	}

	var response []dto.FileListResponseItem
	for _, link := range links {
		response = append(response, dto.FileListResponseItem{
			NodeID:                        link.ChildNodeID.String(),
			Type:                          string(link.ChildNode.Type),
			EncryptedName:                 link.EncryptedName,
			NameNonce:                     link.NameNonce,
			EncryptedNodePassphrase:       link.EncryptedNodePassphrase,
			SignedEncryptedNodePassphrase: link.SignedEncryptedNodePassphrase,
			CreatedAt:                     link.CreatedAt,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
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

func (h *Handler) EmptyTrash(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 1. Get the Authenticated User ID
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
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
		http.Error(w, "failed to query trash", http.StatusInternalServerError)
		return
	}

	if len(trashedLinks) == 0 {
		w.WriteHeader(http.StatusOK)
		return
	}

	go h.wipeTrashedLinks(context.Background(), trashedLinks)

	w.WriteHeader(http.StatusAccepted)
}

func (h *Handler) GetQuota(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// For MVP, we fetch the first ShareMember (mimicking an authenticated user)
	var shareMember model.ShareMember
	if err := h.db.First(&shareMember).Error; err != nil {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}

	var usedBytes int64
	// Because trashed items are NOT soft-deleted in the nodes table, this correctly includes trash!
	h.db.Model(&model.Node{}).
		Where("owner_id = ?", shareMember.UserID).
		Select("COALESCE(SUM(size_bytes), 0)").
		Scan(&usedBytes)

	// 100 MB default quota
	var maxBytes int64 = 100 * 1024 * 1024

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"usedBytes": usedBytes,
		"maxBytes":  maxBytes,
	})
}

func (h *Handler) MoveFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req dto.MoveFileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	// 1. Validate UUIDs
	nodeUUID, err1 := uuid.Parse(req.NodeID)
	oldParentUUID, err2 := uuid.Parse(req.OldParentFolderID)
	newParentUUID, err3 := uuid.Parse(req.NewParentFolderID)
	if err1 != nil || err2 != nil || err3 != nil {
		http.Error(w, "invalid UUIDs", http.StatusBadRequest)
		return
	}

	// 2. Find the exact Link connecting the File to the Old Folder
	var link model.Link
	if err := h.db.Where("child_node_id = ? AND parent_node_id = ?", nodeUUID, oldParentUUID).First(&link).Error; err != nil {
		http.Error(w, "file not found in the specified source folder", http.StatusNotFound)
		return
	}

	// 3. Cryptographic Re-link! Update the parent and overwrite all crypto fields
	link.ParentNodeID = &newParentUUID
	link.EncryptedName = req.NewEncryptedName
	link.NameNonce = req.NewNameNonce
	link.EncryptedNodePassphrase = req.NewEncryptedNodePassphrase
	link.SignedEncryptedNodePassphrase = req.NewSignedEncryptedPassphrase

	// 4. Save the new Link to the database
	if err := h.db.Save(&link).Error; err != nil {
		http.Error(w, "failed to move file", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *Handler) GetAllFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 1. Get the current User ID from the Auth Middleware Context!
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var links []model.Link
	// 2. Fetch every single file/folder the user owns in one highly optimized query
	// We join the `nodes` table so we can filter by the OwnerID!
	err := h.db.Joins("JOIN nodes ON nodes.id = links.child_node_id").
		Where("nodes.owner_id = ?", userID).
		Preload("ChildNode").
		Find(&links).Error

	if err != nil {
		http.Error(w, "failed to fetch files for search index", http.StatusInternalServerError)
		return
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
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (h *Handler) GetFilePath(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	nodeID := r.URL.Query().Get("nodeId")
	if nodeID == "" {
		http.Error(w, "missing nodeId", http.StatusBadRequest)
		return
	}

	// Verify the user is authenticated (ensure they own the nodes later!)
	_, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
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
		http.Error(w, "failed to resolve cryptographic path", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (h *Handler) ShareFolder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	authorID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
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
		http.Error(w, "bad request", http.StatusBadRequest)
		return
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
		http.Error(w, "failed to execute share transaction", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func (h *Handler) GetSharedFolders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// 1. Fetch all SHARED ShareMembers for this user
	var shareMembers []model.ShareMember
	err := h.db.Preload("Share").
		Joins("JOIN shares ON shares.id = share_members.share_id").
		Where("share_members.user_id = ? AND shares.type = ?", userID, model.ShareTypeShared).
		Find(&shareMembers).Error

	if err != nil {
		http.Error(w, "failed to query shares", http.StatusInternalServerError)
		return
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
	json.NewEncoder(w).Encode(response)
}
