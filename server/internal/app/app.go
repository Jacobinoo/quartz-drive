package app

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"log/slog"
	"net/http"
	"quartz/api/account"
	"quartz/api/demo"
	"quartz/api/dev"
	"quartz/api/devices"
	"quartz/api/files"
	"quartz/api/keys"
	"quartz/api/recovery"
	"quartz/api/refresh"
	"quartz/api/signin"
	"quartz/api/signout"
	"quartz/api/signup"
	"quartz/config"
	"quartz/internal/bindings"
	"quartz/internal/middleware"
	"quartz/pkg/database"
	"quartz/pkg/httputils"
	"quartz/pkg/storage"
	"quartz/pkg/worker"
	"strings"
	"sync/atomic"

	"encoding/base64"
	"os"
	"os/signal"
	"syscall"
	"time"

	sentryhttp "github.com/getsentry/sentry-go/http"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/resend/resend-go/v2"
	"gorm.io/gorm"
)

var isDraining atomic.Bool

// MarkDraining is called when SIGTERM is received
func MarkDraining() {
	isDraining.Store(true)
}

func Run() {
	// grpcClient, conn := initGrpcClient()
	// defer conn.Close()

	redisClient := database.NewRedis()
	defer redisClient.Close()

	resendClient := resend.NewClient(config.Cfg.Email.Key)

	db := initDb()
	if db == nil {
		log.Fatal("database initialization failed")
		return
	}

	asynqServer := worker.InitBackgroundWorkerServer(db, resendClient, redisClient)
	asynqClient := worker.InitBackgroundWorkerClient(redisClient)

	var storageService storage.StorageService
	var err error
	if config.Cfg.App.Env == "development" {
		storageService, err = storage.NewS3Service()
	} else {
		storageService, err = storage.NewB2Service()
	}
	if err != nil {
		slog.Error("storage service: connection failed", "error", err)
		os.Exit(1)
		return
	}

	fakeRegRecord, err := bindings.GenerateFakeRegistrationRecord(opaqueSetupBytes())
	if err != nil {
		slog.Error("Failed to generate fake registration record", "error", err)
		os.Exit(1)
		return
	}

	state := &ServerState{
		DB:          db,
		Redis:       redisClient,
		OpaqueSetup: opaqueSetupBytes(),
		// GRPCClient:     *grpcClient,
		// GRPCContext:    context.WithoutCancel(context.Background()),
		StorageService:               storageService,
		AsynqClient:                  asynqClient,
		FakeOpaqueRegistrationRecord: fakeRegRecord,
	}

	router := initRouter(state)

	compressedHandler := chimiddleware.Compress(5)(router)

	cacheNoStoreHandler := middleware.CacheControlNoStoreMiddleware(compressedHandler)

	// Limit request body size for security & abuse prevention
	bodyLimitedHandler := middleware.LimitBodySize(cacheNoStoreHandler)

	finalHandler := httputils.RequestIDMiddleware(bodyLimitedHandler)
	sentryHandler := sentryhttp.New(sentryhttp.Options{})

	server := &http.Server{
		Addr:    config.Cfg.Host + ":" + config.Cfg.Port,
		Handler: sentryHandler.Handle(finalHandler),
	}

	// Start server in a goroutine
	go func() {
		slog.Info("Server running", "host", config.Cfg.Host, "port", config.Cfg.Port)
		var err error
		if config.Cfg.CertFilePath != "" && config.Cfg.KeyFilePath != "" {
			err = server.ListenAndServeTLS(config.Cfg.CertFilePath, config.Cfg.KeyFilePath)
		} else {
			slog.Info("No TLS certificates found, falling back to HTTP")
			err = server.ListenAndServe()
		}

		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server listening error", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	// kill (no param) default send syscanll.SIGTERM
	// kill -2 is syscall.SIGINT
	// kill -9 is syscall.SIGKILL but can't be caught, so don't need add it
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	slog.Info("Shutdown signal received, initiating drain...")

	MarkDraining()

	drainWindow := 3 * time.Second
	drainEnvVal := config.Cfg.App.ShutdownDrainDelay
	if d, err := time.ParseDuration(drainEnvVal); err == nil {
		drainWindow = d
	}

	time.Sleep(drainWindow)

	// The context is used to inform the server it has 5 seconds to finish
	// the request it is currently handling
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		slog.Error("Server forced to shutdown", "error", err)
	}

	asynqServer.Shutdown()

	log.Println("Server exiting")
}

func initRouter(state *ServerState) *http.ServeMux {
	rootMux := http.NewServeMux()
	v1Mux := initV1Mux(state)

	rootMux.HandleFunc("/", rootHandler)
	rootMux.HandleFunc("/health", healthHandler(state))
	rootMux.HandleFunc("/up", upHandler)

	if config.Cfg.App.Env == "development" {
		rootMux.HandleFunc("GET /dev/email-preview/password-reset", httputils.Wrap(dev.PreviewPasswordReset))
		rootMux.HandleFunc("GET /dev/email-preview/signup", httputils.Wrap(dev.PreviewSignupVerification))
		rootMux.HandleFunc("GET /dev/email-preview/signup-exists", httputils.Wrap(dev.PreviewSignupAccountExists))
	}

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
	// In Go's ServeMux, "/" is a wildcard prefix match.
	// If the path isn't exactly "/", return a 404 Not Found.
	if r.URL.Path != "/" {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "not_found",
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=300, s-maxage=3600")
	w.WriteHeader(http.StatusOK)

	raw := config.BuildVersion

	version := config.Cfg.App.Version
	commitHash := "unknown"
	buildNumber := "unknown"
	parts := strings.Split(raw, "-")
	for i := 0; i < len(parts); i++ {
		if parts[i] == "sha" && i+1 < len(parts) {
			commitHash = parts[i+1]
			if len(commitHash) > 7 {
				commitHash = commitHash[:7]
			}
		}
		if parts[i] == "build" && i+1 < len(parts) {
			buildNumber = parts[i+1]
		}
	}
	response := struct {
		Service            string `json:"service"`
		EnvironmentVersion string `json:"environmentVersion"`
		Version            string `json:"version"`
		BuildNumber        string `json:"buildNumber"`
		CommitHash         string `json:"commitHash"`
		Description        string `json:"description"`
	}{
		Service:            config.Cfg.App.Name,
		EnvironmentVersion: config.Cfg.App.Version,
		Version:            version,
		BuildNumber:        buildNumber,
		CommitHash:         commitHash,
		Description:        "Quartz API Server",
	}
	err := json.NewEncoder(w).Encode(&response)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	} //this is public on GET /
}

func healthHandler(state *ServerState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		expectedToken := "Bearer " + config.Cfg.App.HealthToken
		if r.Header.Get("Authorization") != expectedToken {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
			return
		}

		status := "healthy"

		// Check Redis/Valkey
		if state.Redis.Ping(r.Context()).Err() != nil {
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

func upHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=5")

	if isDraining.Load() {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	w.WriteHeader(http.StatusOK)
	return
}

func initV1Mux(state *ServerState) *http.ServeMux {
	mux := http.NewServeMux()

	accountHandler := account.NewHandler(state.DB, state.Redis, state.AsynqClient, state.OpaqueSetup)
	account.RegisterRoutes(mux, accountHandler, state.Redis)

	signinHandler := signin.NewHandler(state.DB, state.Redis, state.OpaqueSetup, state.FakeOpaqueRegistrationRecord)
	signin.RegisterRoutes(mux, signinHandler, state.Redis)

	demoHandler := demo.NewHandler(state.DB, state.Redis, state.AsynqClient, state.StorageService)
	go demoHandler.StartSweepScheduler(context.Background())
	demo.RegisterRoutes(mux, demoHandler)

	signoutHandler := signout.NewHandler(state.DB)
	signout.RegisterRoutes(mux, signoutHandler, state.Redis)

	signupHandler := signup.NewHandler(state.DB, state.Redis, state.OpaqueSetup, state.AsynqClient)
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

	recoveryHandler := recovery.NewHandler(state.DB, state.Redis, state.AsynqClient, state.OpaqueSetup)
	recovery.RegisterRoutes(mux, recoveryHandler)

	return mux
}
