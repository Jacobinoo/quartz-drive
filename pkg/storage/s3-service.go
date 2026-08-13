package storage

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"quartz/config"
	"strconv"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type S3Service struct {
	client *minio.Client
	bucket string
}

var _ StorageService = (*S3Service)(nil)

func NewS3Service() (*S3Service, error) {
	endpoint := config.Cfg.S3.Endpoint               //"localhost:8333"
	accessKeyID := config.Cfg.S3.AccessKeyID         //"SILKW41POES3ATP3DL99"
	secretAccessKey := config.Cfg.S3.SecretAccessKey //"/luBJXPD30oZz5Sjt9Uei2H/6yqg4yLVfahjHlhD"

	customTransport := http.DefaultTransport.(*http.Transport).Clone()
	customTransport.TLSClientConfig = &tls.Config{InsecureSkipVerify: config.Cfg.S3.InsecureSkipVerify}

	minioClient, err := minio.New(endpoint, &minio.Options{
		Creds:     credentials.NewStaticV4(accessKeyID, secretAccessKey, ""),
		Secure:    config.Cfg.S3.Secure,
		Transport: customTransport,
	})
	if err != nil {
		return nil, err
	}

	bucketName := config.Cfg.S3.BucketName

	// Check if the bucket exists, if not - create it
	ctx := context.Background()
	var exists bool
	maxRetries := 30
	for i := 0; i < maxRetries; i++ {
		exists, err = minioClient.BucketExists(ctx, bucketName)
		if err == nil {
			break
		}
		log.Printf("Waiting for S3 to be ready... (attempt %d/%d)", i+1, maxRetries)
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to check bucket existence after retries: %w", err)
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

// Funkcja generująca Upload Presigned URL
func (s *S3Service) GenerateUploadUrl(ctx context.Context, objectKey string, expiry time.Duration, expectedSize int64, chunkHash string) (string, error) {

	extraHeaders := http.Header{}
	extraHeaders.Set("Content-Length", strconv.FormatInt(expectedSize, 10))
	extraHeaders.Set("x-amz-checksum-sha256", chunkHash)

	log.Printf("Generating presigned url for object %s signed with Content-Length %d and checksum sha256 %s", objectKey, expectedSize, chunkHash)

	presignedURL, err := s.client.PresignHeader(
		ctx,
		http.MethodPut,
		s.bucket,
		objectKey,
		expiry,
		nil,
		extraHeaders,
	)
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned upload URL for object %s: %w", objectKey, err)
	}

	return presignedURL.String(), nil
}

// Funkcja generująca Download Presigned URLs
func (s *S3Service) GenerateDownloadUrls(ctx context.Context, objectKeys []string) ([]string, error) {
	var urls []string
	expiry := time.Minute * 10 // Safe expiry for downloads

	for i, objectKey := range objectKeys {
		presignedURL, err := s.client.PresignedGetObject(ctx, s.bucket, objectKey, expiry, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to generate GET URL for chunk %d: %w", i, err)
		}
		urls = append(urls, presignedURL.String())
	}

	return urls, nil
}

// Helper to physically delete a chunk from MinIO
func (s *S3Service) DeleteChunk(ctx context.Context, objectKey string) error {
	return s.client.RemoveObject(ctx, s.bucket, objectKey, minio.RemoveObjectOptions{})
}

// Security: Fetches the physical size of a chunk directly from S3 to prevent client spoofing
func (s *S3Service) GetChunkSize(ctx context.Context, objectKey string) (int64, string, string, error) {
	opts := minio.StatObjectOptions{}
	opts.Set("x-amz-checksum-mode", "ENABLED")

	stat, err := s.client.StatObject(ctx, s.bucket, objectKey, opts)
	if err != nil {
		return 0, "", "", err
	}
	return stat.Size, stat.ETag, stat.ChecksumSHA256, nil
}
