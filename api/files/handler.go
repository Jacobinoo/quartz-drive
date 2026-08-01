package files

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"quartz/internal/dto"
	"quartz/internal/middleware"
	"quartz/internal/model"
	"quartz/pkg/storage"
	"sync"
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

func (h *Handler) Upload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	var uploadRequest dto.InitFileUploadRequest
	if err := json.NewDecoder(r.Body).Decode(&uploadRequest); err != nil {
		http.Error(w, err.Error(), http.StatusAccepted)
		return
	}

	// 2. SPRAWDŹ UPRAWNIENIA (Czy user ma prawo pisać do tego NodeID?)
	// TODO

	// 3. Wygeneruj presigned URLs
	urls, err := h.storage.GenerateUploadUrls(r.Context(), uploadRequest.NodeID, uploadRequest.TotalChunks)
	if err != nil {
		log.Printf(err.Error())
		http.Error(w, "Failed to generate upload links", 500)
		return
	}

	uploadResponse := InitFileUploadResponse{
		PresignedUrls: urls,
	}

	// 4. Zwróć je do frontendu (do web workera)
	err = json.NewEncoder(w).Encode(uploadResponse)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
}

func (h *Handler) FinishUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	var req dto.FinishFileUploadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	nodeUUID, err := uuid.Parse(req.NodeID)
	if err != nil {
		http.Error(w, "invalid nodeId uuid", http.StatusBadRequest)
		return
	}

	parentUUID, err := uuid.Parse(req.ParentNodeID)
	if err != nil {
		http.Error(w, "invalid parentNodeId uuid", http.StatusBadRequest)
		return
	}

	linkUUID := uuid.New()

	// TODO: Extract actual OwnerID from your DPoP/JWT auth claims or session
	// For testing/MVP if not extracted yet, fetch the parent node's owner:
	var parentNode model.Node
	if err := h.db.First(&parentNode, "id = ?", parentUUID).Error; err != nil {
		http.Error(w, "parent folder not found", http.StatusNotFound)
		return
	}
	ownerID := parentNode.OwnerID

	// 1. Create the File Node
	fileNode := model.Node{
		ID:                nodeUUID,
		Type:              model.NodeTypeFile,
		SizeBytes:         req.SizeBytes,
		EncryptedMetadata: req.EncryptedMetadata,
		MetadataNonce:     req.MetadataNonce,
		OwnerID:           ownerID,
		NodePublicKey:     req.NodePublicKey,
		WrappedNodeKey:    req.WrappedNodeKey,
		NodePrivNonce:     req.NodePrivNonce,
		Signature:         req.SignedEncryptedNodePassphrase,
	}

	// 2. Create the Link inside the parent folder
	fileLink := model.Link{
		ID:                            linkUUID,
		ParentNodeID:                  &parentUUID,
		ChildNodeID:                   &nodeUUID,
		EncryptedName:                 req.EncryptedName,
		NameNonce:                     req.NameNonce,
		EncryptedNodePassphrase:       req.EncryptedNodePassphrase,
		SignedEncryptedNodePassphrase: req.SignedEncryptedNodePassphrase,
		AuthorID:                      ownerID,
	}

	// 3. Security Check: Concurrently fetch TRUE chunk sizes directly from S3
	var blocks []model.FileBlock
	var trueTotalSize int64
	var mu sync.Mutex

	var wg sync.WaitGroup
	errCh := make(chan error, req.TotalChunks)

	for i := 0; i < req.TotalChunks; i++ {
		wg.Add(1)
		go func(chunkIndex int) {
			defer wg.Done()

			nonce := ""
			if chunkIndex < len(req.ChunkNonces) {
				nonce = req.ChunkNonces[chunkIndex]
			}

			objectKey := fmt.Sprintf("%s/chunk_%d", req.NodeID, chunkIndex)

			// Verify physical size directly from S3!
			size, err := h.storage.GetChunkSize(r.Context(), objectKey)
			if err != nil {
				errCh <- fmt.Errorf("chunk %d missing in S3: %v", chunkIndex, err)
				return
			}

			mu.Lock()
			trueTotalSize += size
			blocks = append(blocks, model.FileBlock{
				ID:        uuid.New(),
				NodeID:    nodeUUID,
				Index:     chunkIndex,
				Bucket:    "default",
				ObjectKey: objectKey,
				Nonce:     nonce,
				Size:      int(size),
			})
			mu.Unlock()
		}(i)
	}

	wg.Wait()
	close(errCh)

	if len(errCh) > 0 {
		err := <-errCh
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
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
		return nil
	})

	if err != nil {
		log.Printf("Failed to finish file upload in DB: %v", err)
		http.Error(w, "database error while saving file metadata", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"status": "created", "nodeId": req.NodeID})
}

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

	// 2. Generate URLs
	urls, err := h.storage.GenerateDownloadUrls(r.Context(), nodeID, len(blocks))
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
	if err := h.db.Preload("Share").Where("user_id = ?", userID).First(&shareMember).Error; err != nil {
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

	go func(links []model.Link) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		log.Printf("Starting Async S3 Wipe for %d trashed items...", len(links))
		for _, link := range links {
			// 1. Gather the NodeID of the trashed item
			nodeIDsToWipe := []string{link.ChildNodeID.String()}

			// 2. If it's a folder, recursively gather ALL descendants inside it!
			if link.ChildNode.Type == model.NodeTypeFolder {
				descendants, err := h.getDescendantNodeIDs(link.ChildNodeID.String())
				if err != nil {
					log.Printf("Failed to get descendants for folder %s, skipping wipe for this link: %v", link.ChildNodeID, err)
					continue // don't wipe/delete anything for this link if we can't be sure we found everything inside it
				}
				nodeIDsToWipe = append(nodeIDsToWipe, descendants...)
			}

			// 3. Find ALL FileBlocks for ALL these nodes
			var blocks []model.FileBlock
			if err := h.db.Where("node_id IN ?", nodeIDsToWipe).Find(&blocks).Error; err != nil {
				log.Printf("Failed to query file_blocks for nodes %v: %v", nodeIDsToWipe, err)
				continue
			}

			var successfulBlocks []string

			// 4. Wipe from MinIO (No more storage leaks!)
			failedNodeIDs := map[string]bool{}
			for _, block := range blocks {
				if err := h.storage.DeleteChunk(ctx, block.ObjectKey); err != nil {
					log.Printf("Failed to wipe S3 chunk %s: %v", block.ObjectKey, err)
					failedNodeIDs[block.NodeID.String()] = true // adjust field name to your model
					continue
				}
				successfulBlocks = append(successfulBlocks, block.ObjectKey)
			}

			var cleanNodeIDs []string
			for _, id := range nodeIDsToWipe {
				if !failedNodeIDs[id] {
					cleanNodeIDs = append(cleanNodeIDs, id)
				}
			}

			if len(cleanNodeIDs) == 0 {
				continue // nothing safe to clean up for this link, leave it all for the sweep job
			}

			err := h.db.Transaction(func(tx *gorm.DB) error {
				if err := tx.Exec("DELETE FROM file_blocks WHERE node_id IN (?) AND object_key IN (?)", nodeIDsToWipe, successfulBlocks).Error; err != nil {
					return err
				}

				if err := tx.Exec("DELETE FROM links WHERE child_node_id IN (?) OR parent_node_id IN (?)", nodeIDsToWipe, nodeIDsToWipe).Error; err != nil {
					return err
				}

				if err := tx.Exec("DELETE FROM nodes WHERE id IN (?)", nodeIDsToWipe).Error; err != nil {
					return err
				}
				return nil
			})
			if err != nil {
				log.Printf("Failed to complete DB wipe transaction: %v", err)
			}
		}
		log.Println("Async S3 Wipe Complete!")
	}(trashedLinks)

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
