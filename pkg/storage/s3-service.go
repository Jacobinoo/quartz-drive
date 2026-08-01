package storage

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"quartz/config"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type S3Service struct {
	client *minio.Client
	bucket string
}

func NewS3Service() (*S3Service, error) {
	endpoint := config.Cfg.S3.Endpoint               //"localhost:8333"
	accessKeyID := config.Cfg.S3.AccessKeyID         //"SILKW41POES3ATP3DL99"
	secretAccessKey := config.Cfg.S3.SecretAccessKey //"/luBJXPD30oZz5Sjt9Uei2H/6yqg4yLVfahjHlhD"

	customTransport := http.DefaultTransport.(*http.Transport).Clone()
	customTransport.TLSClientConfig = &tls.Config{InsecureSkipVerify: config.Cfg.S3.InsecureSkipVerify}

	minioClient, err := minio.New(endpoint, &minio.Options{
		Creds:     credentials.NewStaticV4(accessKeyID, secretAccessKey, ""),
		Secure:    true,
		Transport: customTransport,
	})
	if err != nil {
		return nil, err
	}

	bucketName := config.Cfg.S3.BucketName

	// Check if the bucket exists, if not - create it
	ctx := context.Background()
	exists, err := minioClient.BucketExists(ctx, bucketName)
	if err != nil {
		return nil, fmt.Errorf("failed to check bucket existence: %w", err)
	}
	if !exists {
		if err := minioClient.MakeBucket(ctx, bucketName, minio.MakeBucketOptions{}); err != nil {
			return nil, fmt.Errorf("failed to create bucket: %w", err)
		}
	}

	return &S3Service{
		client: minioClient,
		bucket: bucketName,
	}, nil
}

// Funkcja generująca Upload Presigned URLs dla X chunków
func (s *S3Service) GenerateUploadUrls(nodeID string, totalChunks int) ([]string, error) {
	var urls []string
	expiry := time.Minute * 15 // Ważność linku to 15 minut

	for i := 0; i < totalChunks; i++ {
		// Budujemy ścieżkę pliku w SeaweedFS, np.: drive-chunks/uuid-wezla/chunk_0
		objectName := fmt.Sprintf("%s/chunk_%d", nodeID, i)

		// Generujemy Presigned URL dla metody PUT
		presignedURL, err := s.client.PresignedPutObject(context.Background(), s.bucket, objectName, expiry)
		if err != nil {
			return nil, fmt.Errorf("failed to generate URL for chunk %d: %w", i, err)
		}

		urls = append(urls, presignedURL.String())
	}

	return urls, nil
}

// Funkcja generująca Download Presigned URLs
func (s *S3Service) GenerateDownloadUrls(nodeID string, totalChunks int) ([]string, error) {
	var urls []string
	expiry := time.Minute * 15

	for i := 0; i < totalChunks; i++ {
		objectName := fmt.Sprintf("%s/chunk_%d", nodeID, i)
		presignedURL, err := s.client.PresignedGetObject(context.Background(), s.bucket, objectName, expiry, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to generate GET URL for chunk %d: %w", i, err)
		}
		urls = append(urls, presignedURL.String())
	}

	return urls, nil
}

// Helper to physically delete a chunk from MinIO
func (s *S3Service) DeleteChunk(objectKey string) error {
	return s.client.RemoveObject(context.Background(), s.bucket, objectKey, minio.RemoveObjectOptions{})
}

// Security: Fetches the physical size of a chunk directly from S3 to prevent client spoofing
func (s *S3Service) GetChunkSize(objectKey string) (int64, error) {
	stat, err := s.client.StatObject(context.Background(), s.bucket, objectKey, minio.StatObjectOptions{})
	if err != nil {
		return 0, err
	}
	return stat.Size, nil
}
