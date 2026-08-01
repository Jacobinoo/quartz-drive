package storage

import "context"

type StorageService interface {
	// Funkcja generująca Upload Presigned URLs dla X chunków
	GenerateUploadUrls(ctx context.Context, nodeID string, totalChunks int) ([]string, error)

	// Funkcja generująca Download Presigned URLs
	GenerateDownloadUrls(ctx context.Context, nodeID string, totalChunks int) ([]string, error)

	// Helper to physically delete a chunk from MinIO
	DeleteChunk(ctx context.Context, objectKey string) error

	// Security: Fetches the physical size of a chunk directly from S3 to prevent client spoofing
	GetChunkSize(ctx context.Context, objectKey string) (int64, error)
}
