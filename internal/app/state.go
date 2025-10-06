package app

import (
	"context"
	pb "quartz/proto"

	"gorm.io/gorm"
)

type ServerState struct {
	DB          *gorm.DB
	GRPCClient  pb.QuartzInternalCryptoServiceClient
	GRPCContext context.Context
}
