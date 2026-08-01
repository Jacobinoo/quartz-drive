package app

import (
	"context"
	"quartz/pkg/storage"
	pb "quartz/proto"

	"gorm.io/gorm"
)

type ServerState struct {
	DB             *gorm.DB
	GRPCClient     pb.QuartzInternalCryptoServiceClient
	GRPCContext    context.Context
	StorageService storage.StorageService
}
