package files_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"quartz/api/files"
	"quartz/internal/dto"
	"quartz/internal/model"
	"quartz/pkg/contextkeys"
	"quartz/pkg/httputils"
)

type mockStorageService struct{}

func (m *mockStorageService) GenerateUploadUrl(ctx context.Context, objectKey string, expiry time.Duration, expectedSize int64, chunkHash string) (string, error) {
	return "http://mock-url.com", nil
}

func (m *mockStorageService) GenerateDownloadUrls(ctx context.Context, objectKeys []string) ([]string, error) {
	return []string{"http://mock-url.com"}, nil
}

func (m *mockStorageService) DeleteChunk(ctx context.Context, objectKey string) error {
	return nil
}

func (m *mockStorageService) GetChunkSize(ctx context.Context, objectKey string) (int64, string, string, error) {
	return 100, "mock-hash", "mock-sig", nil
}

func (m *mockStorageService) CopyChunk(ctx context.Context, srcObjectKey string, destObjectKey string) error {
	return nil
}

func (m *mockStorageService) ApplyLegalHold(ctx context.Context, objectKey string, status bool) error {
	return nil
}

func setupTestRedis(t *testing.T) *redis.Client {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	return redis.NewClient(&redis.Options{Addr: mr.Addr()})
}

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect database: %v", err)
	}

	err = db.AutoMigrate(&model.User{}, &model.Volume{}, &model.Node{}, &model.FileBlock{}, &model.Upload{}, &model.UploadChunk{})
	if err != nil {
		t.Fatalf("failed to migrate database %v", err)
	}

	return db
}

func TestUploadInit_BOLA_CrossVolumeAccess(t *testing.T) {
	db := setupTestDB(t)
	rdb := setupTestRedis(t)

	handler := files.NewHandler(db, &mockStorageService{}, rdb)

	userA_ID := uuid.New()
	userA := model.User{ID: userA_ID, EncryptedEmail: "usera@test.com", HashedEmail: "a"}
	db.Create(&userA)

	volumeA := model.Volume{ID: uuid.New(), Type: model.VolumeTypePrivate, OwnerUserID: &userA_ID, StorageQuota: 1000, RootNodeID: uuid.New()}
	db.Create(&volumeA)

	nodeA := model.Node{ID: uuid.New(), OwnerID: userA_ID, VolumeID: volumeA.ID, Type: model.NodeTypeFolder}
	db.Create(&nodeA)

	userB_ID := uuid.New()
	userB := model.User{ID: userB_ID, EncryptedEmail: "userb@test.com", HashedEmail: "b"}
	db.Create(&userB)

	volumeB := model.Volume{ID: uuid.New(), Type: model.VolumeTypePrivate, OwnerUserID: &userB_ID, StorageQuota: 1000, RootNodeID: uuid.New()}
	db.Create(&volumeB)

	// Simulate User B trying to upload a file into User A's folder
	uploadReq := dto.InitFileUploadRequest{
		ParentNodeID:  nodeA.ID.String(),
		TotalChunks:   1,
		TotalFileSize: 500,
	}
	body, _ := json.Marshal(uploadReq)

	req := httptest.NewRequest("POST", "/v1/files/upload/init", bytes.NewReader(body))
	// Inject User B's ID to simulate B being logged in (fool the access token check)
	ctx := context.WithValue(req.Context(), contextkeys.UserIDKey, userB_ID)
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()

	wrappedHandler := httputils.Wrap(func(w http.ResponseWriter, r *http.Request) error {
		return handler.InitUpload(w, r)
	})

	wrappedHandler.ServeHTTP(rr, req)

	if rr.Code == http.StatusOK {
		t.Fatalf("BOLA Vulnerability! User B was able to initialize an upload in User A's folder.")
	}
	if rr.Code != http.StatusNotFound && rr.Code != http.StatusForbidden {
		t.Errorf("Expected 404 or 403, got %d", rr.Code)
	}
}
