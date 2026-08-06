package files

import (
	"time"

	"github.com/google/uuid"
)

type InitFileUploadResponse struct {
	UploadID  uuid.UUID `json:"uploadId"`
	NodeID    uuid.UUID `json:"nodeId"`
	ExpiresAt time.Time `json:"expiresAt"`
}
