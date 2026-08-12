package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"quartz/api/account"
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
	"os"
	"os/signal"
	"syscall"
	"time"

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
		DB:          db,
		Redis:       redisClient,
		OpaqueSetup: opaqueSetupBytes(),
		// GRPCClient:     *grpcClient,
		// GRPCContext:    context.WithoutCancel(context.Background()),
		StorageService: storageService,
	}

	router := initRouter(state)

	server := &http.Server{
		Addr:    config.Cfg.Host + ":" + config.Cfg.Port,
		Handler: router,
	}

	// Start server in a goroutine
	go func() {
		fmt.Printf("Server running on %s:%s\n", config.Cfg.Host, config.Cfg.Port)
		if err := server.ListenAndServeTLS(config.Cfg.CertFilePath, config.Cfg.KeyFilePath); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %s\n", err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	// kill (no param) default send syscanll.SIGTERM
	// kill -2 is syscall.SIGINT
	// kill -9 is syscall.SIGKILL but can't be caught, so don't need add it
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	// The context is used to inform the server it has 5 seconds to finish
	// the request it is currently handling
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatal("Server forced to shutdown: ", err)
	}

	log.Println("Server exiting")
}

func initRouter(state *ServerState) *http.ServeMux {
	rootMux := http.NewServeMux()
	v1Mux := initV1Mux(state)

	rootMux.HandleFunc("/", rootHandler)
	rootMux.HandleFunc("/health", healthHandler(state))
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
	// In Go's ServeMux, "/" is a wildcard prefix match.
	// If the path isn't exactly "/", return a 404 Not Found.
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	response := struct {
		Service     string
		Version     string
		Description string
	}{
		Service:     config.Cfg.App.Name,
		Version:     config.Cfg.App.Version,
		Description: "Quartz API Server",
	}

	json.NewEncoder(w).Encode(&response) //this is public on GET /
}

func healthHandler(state *ServerState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Require Bearer token
		expectedToken := "Bearer " + config.Cfg.App.HealthToken
		if r.Header.Get("Authorization") != expectedToken {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
			return
		}

		status := "healthy"

		// Check Database
		sqlDB, err := state.DB.DB()
		if err != nil || sqlDB.Ping() != nil {
			status = "unhealthy (db)"
			w.WriteHeader(http.StatusServiceUnavailable)
		} else if state.Redis.Ping(r.Context()).Err() != nil {
			// Check Redis/Valkey
			status = "unhealthy (redis)"
			w.WriteHeader(http.StatusServiceUnavailable)
		} else {
			w.WriteHeader(http.StatusOK)
		}

		json.NewEncoder(w).Encode(map[string]string{
			"status": status,
			"time":   time.Now().UTC().Format(time.RFC3339),
		})
	}
}

func initV1Mux(state *ServerState) *http.ServeMux {
	mux := http.NewServeMux()

	accountHandler := account.NewHandler(state.DB, state.Redis, state.OpaqueSetup)
	account.RegisterRoutes(mux, accountHandler, state.Redis)

	signinHandler := signin.NewHandler(state.DB, state.Redis, state.OpaqueSetup)
	signin.RegisterRoutes(mux, signinHandler, state.Redis)

	signoutHandler := signout.NewHandler(state.DB)
	signout.RegisterRoutes(mux, signoutHandler, state.Redis)

	signupHandler := signup.NewHandler(state.DB, state.Redis, state.OpaqueSetup)
	signup.RegisterRoutes(mux, signupHandler, state.Redis)

	refreshHandler := refresh.NewHandler(state.DB)
	refresh.RegisterRoutes(mux, refreshHandler, state.Redis)

	filesHandler := files.NewHandler(state.DB, state.StorageService, state.Redis)
	go filesHandler.StartSweepScheduler(context.Background())
	files.RegisterRoutes(mux, filesHandler, state.Redis)

	devicesHandler := devices.NewHandler(state.DB)
	devices.RegisterRoutes(mux, devicesHandler, state.Redis)

	keysHandler := keys.NewHandler(state.DB)
	keys.RegisterRoutes(mux, keysHandler, state.Redis)

	return mux
}
