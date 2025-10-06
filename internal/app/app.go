package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"quartz/api/refresh"
	"quartz/api/signin"
	"quartz/api/signup"
	"quartz/config"
	"quartz/pkg/database"
	pb "quartz/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gorm.io/gorm"
)

func Run() {
	grpcClient, conn := initGrpcClient()
	defer conn.Close()

	db := initDb()
	if db == nil {
		log.Fatal("database initialization failed")
		return
	}

	state := &ServerState{
		DB:          db,
		GRPCClient:  *grpcClient,
		GRPCContext: context.WithoutCancel(context.Background()),
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

func initGrpcClient() (*pb.QuartzInternalCryptoServiceClient, *grpc.ClientConn) {
	var opts []grpc.DialOption
	opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))

	conn, grpcErr := grpc.NewClient(config.Cfg.GRPC.Host+":"+config.Cfg.GRPC.Port, opts...)
	if grpcErr != nil {
		log.Fatalf("grpc did not connect: %v", grpcErr)
	}

	grpcClient := pb.NewQuartzInternalCryptoServiceClient(conn)
	return &grpcClient, conn
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(config.Cfg.App)
}

func initV1Mux(state *ServerState) *http.ServeMux {
	mux := http.NewServeMux()

	signinHandler := signin.NewHandler(state.DB, state.GRPCClient, state.GRPCContext)
	signin.RegisterRoutes(mux, signinHandler)

	signupHandler := signup.NewHandler(state.DB, state.GRPCClient, state.GRPCContext)
	signup.RegisterRoutes(mux, signupHandler)

	refreshHandler := refresh.NewHandler(state.DB)
	refresh.RegisterRoutes(mux, refreshHandler)

	return mux
}
