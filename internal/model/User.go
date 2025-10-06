package model

import (
	"quartz/internal/dto"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type User struct {
	gorm.Model
	ID uuid.UUID `gorm:"primarykey;type:uuid"`
	dto.M3
}
