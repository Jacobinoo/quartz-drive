package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"quartz/api/devices"
	"quartz/api/files"
	"quartz/api/keys"
	"quartz/api/refresh"
	"quartz/api/signin"
	"quartz/api/signout"
	"quartz/api/signup"
	"quartz/config"
	"quartz/pkg/database"
	"quartz/pkg/storage"

	"encoding/base64"

	// "google.golang.org/grpc"
	// "google.golang.org/grpc/credentials/insecure"
	"gorm.io/gorm"
)

func Run() {
	// grpcClient, conn := initGrpcClient()
	// defer conn.Close()

	redisClient := database.NewRedis()
	defer redisClient.Close()

	db := initDb()
	if db == nil {
		log.Fatal("database initialization failed")
		return
	}

	storageService, err := storage.NewB2Service()
	if err != nil {
		log.Fatal("storage service: connection failed")
	}

	state := &ServerState{
		DB:             db,
		Redis:          redisClient,
		OpaqueSetup:    opaqueSetupBytes(),
		// GRPCClient:     *grpcClient,
		// GRPCContext:    context.WithoutCancel(context.Background()),
		StorageService: storageService,
	}

	router := initRouter(state)

	fmt.Printf("Server running on %s:%s", config.Cfg.Host, config.Cfg.Port)
	log.Fatal(http.ListenAndServeTLS(config.Cfg.Host+":"+config.Cfg.Port, config.Cfg.CertFilePath, config.Cfg.KeyFilePath, router))
}

func initRouter(state *ServerState) *http.ServeMux {
	rootMux := http.NewServeMux()
	v1Mux := initV1Mux(state)

	rootMux.HandleFunc("/", rootHandler)
	rootMux.Handle("/v1/", http.StripPrefix("/v1", v1Mux))

	return rootMux
}

func initDb() *gorm.DB {
	db, err := database.New()
	if err != nil {
		log.Fatalf("failed to connect database: %v", err)
		return nil
	}
	return db
}

// func initGrpcClient() (*pb.QuartzInternalCryptoServiceClient, *grpc.ClientConn) {
// 	var opts []grpc.DialOption
// 	opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
// 
// 	conn, grpcErr := grpc.NewClient(config.Cfg.GRPC.Host+":"+config.Cfg.GRPC.Port, opts...)
// 	if grpcErr != nil {
// 		log.Fatalf("grpc did not connect: %v", grpcErr)
// 	}
// 
// 	grpcClient := pb.NewQuartzInternalCryptoServiceClient(conn)
// 	return &grpcClient, conn
// }

func opaqueSetupBytes() []byte {
	str := config.Cfg.CRYPTO.OpaqueServerSetup
	bytes, err := base64.StdEncoding.DecodeString(str)
	if err != nil {
		// fallback to URLEncoding if standard fails
		bytes, err = base64.URLEncoding.WithPadding(base64.NoPadding).DecodeString(str)
		if err != nil {
			log.Fatalf("failed to decode OPAQUE_SERVER_SETUP: %v", err)
		}
	}
	if len(bytes) != 128 {
		log.Fatalf("OPAQUE_SERVER_SETUP has invalid length %d (expected 128 bytes)", len(bytes))
	}
	return bytes
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(config.Cfg.App)
}

func initV1Mux(state *ServerState) *http.ServeMux {
	mux := http.NewServeMux()

	signinHandler := signin.NewHandler(state.DB, state.Redis, state.OpaqueSetup)
	signin.RegisterRoutes(mux, signinHandler)

	signoutHandler := signout.NewHandler(state.DB)
	signout.RegisterRoutes(mux, signoutHandler)

	signupHandler := signup.NewHandler(state.DB, state.Redis, state.OpaqueSetup)
	signup.RegisterRoutes(mux, signupHandler)

	refreshHandler := refresh.NewHandler(state.DB)
	refresh.RegisterRoutes(mux, refreshHandler)

	filesHandler := files.NewHandler(state.DB, state.StorageService)
	go filesHandler.StartSweepScheduler(context.Background())
	files.RegisterRoutes(mux, filesHandler)

	devicesHandler := devices.NewHandler(state.DB)
	devices.RegisterRoutes(mux, devicesHandler)

	keysHandler := keys.NewHandler(state.DB)
	keys.RegisterRoutes(mux, keysHandler)

	return mux
}
