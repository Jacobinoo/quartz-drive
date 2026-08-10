package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func main() {
	endpoint := "localhost:9000" // MinIO default
	accessKeyID := "minioadmin"
	secretAccessKey := "minioadmin"

	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKeyID, secretAccessKey, ""),
		Secure: false,
	})
	if err != nil {
		log.Fatalln(err)
	}

	bucketName := "default" // QuartzAccountGo uses "default" bucket
	objectName := "test_79_bytes"

	// Create 49 byte payload
	payload := bytes.Repeat([]byte("A"), 49)

	// Presign URL
	presignedURL, err := client.PresignHeader(context.Background(), http.MethodPut, bucketName, objectName, time.Hour, nil, nil)
	if err != nil {
		log.Fatalln(err)
	}

	// Upload using HTTP PUT
	req, err := http.NewRequest(http.MethodPut, presignedURL.String(), bytes.NewReader(payload))
	if err != nil {
		log.Fatalln(err)
	}
	
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatalln(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Fatalf("Failed to upload: %s", resp.Status)
	}

	// Check stat
	stat, err := client.StatObject(context.Background(), bucketName, objectName, minio.StatObjectOptions{})
	if err != nil {
		log.Fatalln(err)
	}

	fmt.Printf("Uploaded 49 bytes. Stat size: %d\n", stat.Size)
}
