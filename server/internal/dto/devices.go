package dto

import (
	"time"

	"github.com/google/uuid"
)

type DeviceResponse struct {
	SessionID    uuid.UUID `json:"sessionId"`
	DeviceName   string    `json:"deviceName"`
	UserAgent    string    `json:"userAgent"`
	LastActiveAt time.Time `json:"lastActiveAt"`
	CreatedAt    time.Time `json:"createdAt"`
	IsCurrent    bool      `json:"isCurrent"`
}

type RevokeDeviceRequest struct {
	SessionID uuid.UUID `json:"sessionId"`
}
