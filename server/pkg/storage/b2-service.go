package storage

import (
	"context"
	"log"
	"quartz/config"

	"github.com/Backblaze/blazer/b2"
)

type B2Service struct {
	*S3Service
	B2NativeClient    *b2.Client
	B2BucketReference *b2.Bucket
}

var _ StorageService = (*B2Service)(nil)

func NewB2Service() (*B2Service, error) {
	s3, err := NewS3Service()
	if err != nil {
		return nil, err
	}

	ctx := context.Background()

	id := config.Cfg.B2.ApplicationKeyID
	key := config.Cfg.B2.ApplicationKey

	// b2_authorize_account
	b2Client, err := b2.NewClient(ctx, id, key)
	if err != nil {
		log.Fatalln(err)
	}

	bucket, err := b2Client.Bucket(ctx, config.Cfg.S3.BucketName)
	if err != nil {
		log.Fatalln(err)
	}

	log.Printf("b2 service: connected and bucket %s exists", bucket.Name())

	return &B2Service{
		S3Service:         s3,
		B2NativeClient:    b2Client,
		B2BucketReference: bucket,
	}, nil
}

// DeleteChunk overrides S3Service's version — real permanent delete via native API,
// bypassing B2's S3-compatible versioning/hide-marker behavior entirely.
func (b *B2Service) DeleteChunk(ctx context.Context, objectKey string) error {
	err := b.B2BucketReference.Object(objectKey).Delete(ctx)
	if err != nil {
		return err
	}
	return nil
}
