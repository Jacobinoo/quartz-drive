package storage

import (
	"context"
	"time"
)

type StorageService interface {
	// Funkcja generująca Upload Presigned URL
	GenerateUploadUrl(ctx context.Context, objectKey string, expiry time.Duration, expectedSize int64, chunkHash string) (string, error)

	// Funkcja generująca Download Presigned URLs
	GenerateDownloadUrls(ctx context.Context, objectKeys []string) ([]string, error)

	// Helper to physically delete a chunk from MinIO
	DeleteChunk(ctx context.Context, objectKey string) error

	// Security: Fetches the physical size of a chunk directly from S3 to prevent client spoofing
	GetChunkSize(ctx context.Context, objectKey string) (int64, string, string, error)
}
