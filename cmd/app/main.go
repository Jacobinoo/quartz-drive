package main

import (
	"fmt"
	"log"
	"os"
	"quartz/config"
	"quartz/internal/app"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/joho/godotenv"
)

func main() {
	// Check if APP_ENV is set (e.g., development, production)
	appEnv := os.Getenv("APP_ENV")
	envFile := ".env"
	if appEnv != "" {
		envFile = ".env." + appEnv
	}

	// If PORT is already set by systemd, skip loading the .env file so we don't accidentally override it
	if os.Getenv("PORT") == "" {
		err := godotenv.Load(envFile)
		if err != nil && appEnv != "" {
			// If .env.development fails, fallback to standard .env
			log.Printf("Could not load %s, falling back to .env", envFile)
			godotenv.Load(".env")
		} else if err != nil {
			log.Printf("No .env file found. Falling back to system environment variables.")
		}
	}

	config.Cfg.Init()

	if config.Cfg.App.SentryDSN != "" {
		err := sentry.Init(sentry.ClientOptions{
			Dsn:              config.Cfg.App.SentryDSN,
			Environment:      config.Cfg.App.Env,
			Release:          config.Cfg.App.Version,
			TracesSampleRate: 1.0,
		})
		if err != nil {
			log.Fatalf("sentry.Init: %s", err)
		}
		defer sentry.Flush(2 * time.Second)
		fmt.Println("Sentry initialized successfully.")
	}

	app.Run()
}
