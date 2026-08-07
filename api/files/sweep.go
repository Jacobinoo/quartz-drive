package files

import (
	"bufio"
	"context"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"quartz/config"
	"quartz/internal/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var sweepLog *log.Logger

func initSweepLogger() {
	if sweepLog != nil {
		return
	}
	f, err := os.OpenFile("sweeps.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Printf("Failed to open sweeps.log: %v", err)
		sweepLog = log.New(os.Stdout, "\nSWEEP: ", log.Ldate|log.Ltime|log.Lshortfile)
		return
	}
	mw := io.MultiWriter(os.Stdout, f)
	sweepLog = log.New(mw, "\nSWEEP: ", log.Ldate|log.Ltime|log.Lshortfile)
}

func (h *Handler) StartSweepScheduler(ctx context.Context) {
	if config.Cfg.Sweeper.Disabled {
		log.Println("Sweeper is disabled via config")
		return
	}

	initSweepLogger()
	sweepLog.Println("Starting Sweep Scheduler...")

	hourlyDuration, err := time.ParseDuration(config.Cfg.Sweeper.HourlyInterval)
	if err != nil {
		hourlyDuration = 1 * time.Hour
	}
	dailyDuration, err := time.ParseDuration(config.Cfg.Sweeper.DailyInterval)
	if err != nil {
		dailyDuration = 24 * time.Hour
	}
	completedDuration, err := time.ParseDuration(config.Cfg.Sweeper.CompletedInterval)
	if err != nil {
		completedDuration = 1 * time.Hour
	}

	hourly := time.NewTicker(hourlyDuration)
	daily := time.NewTicker(dailyDuration)
	completedTicker := time.NewTicker(completedDuration)

	go func() {
		for {
			select {
			case <-hourly.C:
				sweepLog.Println("Hourly sweep triggered")
				h.SweepExpiredUploads(ctx)
				h.SweepOversizedChunks(ctx)
			case <-completedTicker.C:
				sweepLog.Println("Completed sessions sweep triggered")
				h.SweepCompletedUploads(ctx)
			case <-daily.C:
				sweepLog.Println("Running daily sweeps...")
				h.ReportFlaggedSessions()
				h.SweepReconciliation(ctx)
				h.SweepTrashRetention(ctx)
			case <-ctx.Done():
				sweepLog.Println("Sweep scheduler stopped")
				hourly.Stop()
				daily.Stop()
				completedTicker.Stop()
				return
			}
		}
	}()

	// Debug/Manual Trigger via Console Stdin
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			text := strings.TrimSpace(scanner.Text())
			switch text {
			case "sweep:expired":
				sweepLog.Println("Manual trigger: sweep:expired")
				h.SweepExpiredUploads(ctx)
			case "sweep:completed":
				sweepLog.Println("Manual trigger: sweep:completed")
				h.SweepCompletedUploads(ctx)
			case "sweep:oversized":
				sweepLog.Println("Manual trigger: sweep:oversized")
				h.SweepOversizedChunks(ctx)
			case "sweep:flagged":
				sweepLog.Println("Manual trigger: sweep:flagged")
				h.ReportFlaggedSessions()
			case "sweep:recon":
				sweepLog.Println("Manual trigger: sweep:recon")
				h.SweepReconciliation(ctx)
			case "sweep:trash":
				sweepLog.Println("Manual trigger: sweep:trash")
				h.SweepTrashRetention(ctx)
			case "sweep:all":
				sweepLog.Println("Manual trigger: sweep:all")
				h.SweepExpiredUploads(ctx)
				h.SweepCompletedUploads(ctx)
				h.SweepOversizedChunks(ctx)
				h.ReportFlaggedSessions()
				h.SweepReconciliation(ctx)
				h.SweepTrashRetention(ctx)
			}
		}
	}()
}

func (h *Handler) SweepExpiredUploads(ctx context.Context) {
	sweepLog.Println("SweepExpiredUploads started")
	var expiredUploads []model.Upload
	if err := h.db.Where("expires_at < ? AND status != ?", time.Now(), model.UploadStatusComplete).
		Find(&expiredUploads).Error; err != nil {
		sweepLog.Printf("SweepExpiredUploads error: failed to query expired uploads: %v", err)
		return
	}

	if len(expiredUploads) == 0 {
		sweepLog.Println("SweepExpiredUploads finished (0 sessions)")
		return
	}

	for _, upload := range expiredUploads {
		var chunks []model.UploadChunk
		h.db.Where("upload_id = ?", upload.ID).Find(&chunks)

		hasError := false
		for _, chunk := range chunks {
			if chunk.Status == model.ChunkUploadStatusVerified || chunk.Status == model.ChunkUploadStatusPending {
				if err := h.storage.DeleteChunk(ctx, chunk.ObjectKey); err != nil {
					if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "NoSuchKey") {
						// It's normal for pending chunks to not exist in S3 if they were never uploaded
						continue
					}
					sweepLog.Printf("failed to delete chunk %s: %v", chunk.ObjectKey, err)
					hasError = true
				}
			}
		}

		if hasError {
			sweepLog.Printf("skipping DB wipe for session %s due to S3 errors (will retry next sweep)", upload.ID)
			continue
		}

		h.db.Unscoped().Where("upload_id = ?", upload.ID).Delete(&model.UploadChunk{})
		h.db.Unscoped().Delete(&upload)
	}
	sweepLog.Printf("SweepExpiredUploads finished: cleaned up %d expired upload sessions", len(expiredUploads))
}

func (h *Handler) SweepCompletedUploads(ctx context.Context) {
	sweepLog.Println("SweepCompletedUploads started")
	var completedUploads []model.Upload
	if err := h.db.Where("status = ?", model.UploadStatusComplete).
		Find(&completedUploads).Error; err != nil {
		sweepLog.Printf("SweepCompletedUploads error: failed to query completed uploads: %v", err)
		return
	}

	if len(completedUploads) == 0 {
		sweepLog.Println("SweepCompletedUploads finished (0 sessions)")
		return
	}

	// For completed uploads, the S3 objects are actively used as FileBlocks!
	// We ONLY want to clean up the DB rows, DO NOT delete from S3 here.
	for _, upload := range completedUploads {
		h.db.Unscoped().Where("upload_id = ?", upload.ID).Delete(&model.UploadChunk{})
		h.db.Unscoped().Delete(&upload)
	}

	sweepLog.Printf("SweepCompletedUploads finished: cleaned up %d legacy completed upload sessions from DB", len(completedUploads))
}

func (h *Handler) SweepOversizedChunks(ctx context.Context) {
	sweepLog.Println("SweepOversizedChunks started")
	const maxChunkSize = 4*1024*1024 + 64 // 4MiB + AEAD overhead

	var pendingChunks []model.UploadChunk
	h.db.Where("status = ?", model.ChunkUploadStatusPending).Find(&pendingChunks)

	count := 0
	for _, chunk := range pendingChunks {
		size, _, _, err := h.storage.GetChunkSize(ctx, chunk.ObjectKey)
		if err != nil {
			continue // object doesn't exist yet, not uploaded — normal, skip
		}
		if size > maxChunkSize {
			sweepLog.Printf("found oversized orphan chunk %s: %d bytes", chunk.ObjectKey, size)
			h.storage.DeleteChunk(ctx, chunk.ObjectKey)
			chunk.Status = model.ChunkUploadStatusFlagged
			h.db.Save(&chunk)
			count++
		}
	}
	if count > 0 {
		sweepLog.Printf("SweepOversizedChunks finished: flagged and deleted %d oversized orphan chunks", count)
	} else {
		sweepLog.Println("SweepOversizedChunks finished (0 found)")
	}
}

func (h *Handler) ReportFlaggedSessions() {
	sweepLog.Println("ReportFlaggedSessions started")
	var flagged []model.Upload
	h.db.Where("status = ?", model.UploadStatusFlaggedMalicious).
		Find(&flagged)

	if len(flagged) > 0 {
		sweepLog.Printf("ReportFlaggedSessions finished: DAILY REPORT: %d sessions flagged as malicious total", len(flagged))
		for _, f := range flagged {
			sweepLog.Printf("  - session %s, user %s", f.ID, f.UserID)
		}
	} else {
		sweepLog.Println("ReportFlaggedSessions finished (0 flagged)")
	}
}

func (h *Handler) SweepReconciliation(ctx context.Context) {
	sweepLog.Println("SweepReconciliation started")
	var blocks []model.FileBlock
	h.db.Find(&blocks)

	count := 0
	for _, block := range blocks {
		if _, _, _, err := h.storage.GetChunkSize(ctx, block.ObjectKey); err != nil {
			sweepLog.Printf("RECONCILIATION: FileBlock %s references missing object %s", block.ID, block.ObjectKey)
			count++
		}
	}
	if count > 0 {
		sweepLog.Printf("SweepReconciliation finished: found %d missing objects referenced by DB", count)
	} else {
		sweepLog.Println("SweepReconciliation finished (0 missing objects)")
	}
}

func (h *Handler) wipeTrashedLinks(ctx context.Context, links []model.Link) {
	sweepLog.Printf("Starting Async S3 Wipe for %d trashed items...", len(links))

	// Track which S3 object keys have already been deleted and accounted for in
	// the storage decrement. This prevents double-counting when multiple links in
	// the same batch share descendants (e.g. File1 is a child of both Folder A and
	// Folder B which are both being wiped).
	accountedBlocks := map[string]bool{}

	for _, link := range links {
		nodeIDsToWipe := []string{link.ChildNodeID.String()}

		if link.ChildNode.Type == model.NodeTypeFolder {
			descendants, err := h.getDescendantNodeIDs(link.ChildNodeID.String())
			if err != nil {
				sweepLog.Printf("Failed to get descendants for folder %s, skipping wipe for this link: %v", link.ChildNodeID, err)
				continue
			}
			nodeIDsToWipe = append(nodeIDsToWipe, descendants...)
		}

		var blocks []model.FileBlock
		if err := h.db.Where("node_id IN ?", nodeIDsToWipe).Find(&blocks).Error; err != nil {
			sweepLog.Printf("Failed to query file_blocks for nodes %v: %v", nodeIDsToWipe, err)
			continue
		}

		var nodes []model.Node
		if err := h.db.Where("id IN ?", nodeIDsToWipe).Find(&nodes).Error; err != nil {
			sweepLog.Printf("Failed to query nodes %v: %v", nodeIDsToWipe, err)
			continue
		}
		nodeOwners := make(map[string]uuid.UUID)
		for _, n := range nodes {
			nodeOwners[n.ID.String()] = n.OwnerID
		}

		freedPerOwner := make(map[uuid.UUID]int64)
		var successfulBlocks []string
		failedNodeIDs := map[string]bool{}
		for _, block := range blocks {
			if err := h.storage.DeleteChunk(ctx, block.ObjectKey); err != nil {
				if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "NoSuchKey") {
					// If it's already gone from S3, consider it a successful wipe
					successfulBlocks = append(successfulBlocks, block.ObjectKey)
					if !accountedBlocks[block.ObjectKey] {
						if owner, ok := nodeOwners[block.NodeID.String()]; ok {
							freedPerOwner[owner] += int64(block.Size)
						}
						accountedBlocks[block.ObjectKey] = true
					}
					continue
				}
				sweepLog.Printf("Failed to wipe S3 chunk %s: %v", block.ObjectKey, err)
				failedNodeIDs[block.NodeID.String()] = true
				continue
			}
			successfulBlocks = append(successfulBlocks, block.ObjectKey)
			if !accountedBlocks[block.ObjectKey] {
				if owner, ok := nodeOwners[block.NodeID.String()]; ok {
					freedPerOwner[owner] += int64(block.Size)
				}
				accountedBlocks[block.ObjectKey] = true
			}
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
			if len(successfulBlocks) > 0 {
				// We can safely delete individual file blocks that succeeded, even if other blocks for the node failed.
				if err := tx.Exec("DELETE FROM file_blocks WHERE object_key IN (?)", successfulBlocks).Error; err != nil {
					return err
				}

				// Decrement storage quota safely (atomic)
				for ownerID, sizeFreed := range freedPerOwner {
					if sizeFreed > 0 {
						if err := tx.Model(&model.User{}).Where("id = ?", ownerID).UpdateColumn("storage_used", gorm.Expr("storage_used - ?", sizeFreed)).Error; err != nil {
							return err
						}
					}
				}
			}

			// ONLY delete links and nodes that are completely clean (no failed S3 chunk deletions)
			if err := tx.Exec("DELETE FROM links WHERE child_node_id IN (?) OR parent_node_id IN (?)", cleanNodeIDs, cleanNodeIDs).Error; err != nil {
				return err
			}

			if err := tx.Exec("DELETE FROM nodes WHERE id IN (?)", cleanNodeIDs).Error; err != nil {
				return err
			}

			return nil
		})

		if err != nil {
			sweepLog.Printf("DB wipe transaction failed for link %s: %v", link.ID, err)
		}
	}
	sweepLog.Println("Async S3 Wipe finished.")
}

func (h *Handler) SweepTrashRetention(ctx context.Context) {
	sweepLog.Println("SweepTrashRetention started")
	retentionDays := config.Cfg.Sweeper.TrashRetentionDays
	if retentionDays <= 0 {
		retentionDays = 30
	}

	var oldTrashedLinks []model.Link
	if err := h.db.Unscoped().
		Preload("ChildNode").
		Where("deleted_at < ?", time.Now().Add(time.Duration(-retentionDays)*24*time.Hour)).
		Find(&oldTrashedLinks).Error; err != nil {
		sweepLog.Printf("SweepTrashRetention error: failed to query trashed links: %v", err)
		return
	}

	if len(oldTrashedLinks) > 0 {
		sweepLog.Printf("Found %d old trashed links to permanently wipe", len(oldTrashedLinks))
		h.wipeTrashedLinks(ctx, oldTrashedLinks)
		sweepLog.Println("SweepTrashRetention finished: wiped old trashed links")
	} else {
		sweepLog.Println("SweepTrashRetention finished (0 old links)")
	}
}
