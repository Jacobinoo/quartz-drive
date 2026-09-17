package app

import (
	"quartz/pkg/storage"

	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"

	"gorm.io/gorm"
)

type ServerState struct {
	DB          *gorm.DB
	Redis       *redis.Client
	OpaqueSetup []byte
	// GRPCClient     pb.QuartzInternalCryptoServiceClient
	// GRPCContext    context.Context
	StorageService               storage.StorageService
	AsynqClient                  *asynq.Client
	FakeOpaqueRegistrationRecord []byte
}
