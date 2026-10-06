package main

import (
	"context"
	"log/slog"
	"os"
	"quartz/config"
	"quartz/internal/app"
	"quartz/pkg/logger"
	"time"

	"github.com/getsentry/sentry-go"
	infisical "github.com/infisical/go-sdk"
	"github.com/joho/godotenv"
)

func loadInfisicalSecrets() {
	identityID := os.Getenv("INFISICAL_MACHINE_IDENTITY_ID")
	projectID := os.Getenv("INFISICAL_PROJECT_ID")

	if identityID == "" || projectID == "" {
		slog.Debug("INFISICAL_MACHINE_IDENTITY_ID or INFISICAL_PROJECT_ID not set, skipping Infisical injection")
		return
	}

	apiURL := os.Getenv("INFISICAL_API_URL")
	if apiURL == "" {
		apiURL = "https://app.infisical.com"
	}

	env := os.Getenv("INFISICAL_ENVIRONMENT")
	if env == "" {
		env = "prod"
	}

	slog.Info("Authenticating with Infisical via GCP ID Token...", "url", apiURL, "env", env)

	client := infisical.NewInfisicalClient(context.Background(), infisical.Config{
		SiteUrl: apiURL,
	})

	_, err := client.Auth().GcpIdTokenAuthLogin(identityID)
	if err != nil {
		slog.Error("Failed to authenticate with Infisical GCP ID Token", "error", err)
		os.Exit(1)
	}

	secrets, err := client.Secrets().List(infisical.ListSecretsOptions{
		ProjectID:          projectID,
		Environment:        env,
		SecretPath:         "/",
		AttachToProcessEnv: true,
	})
	if err != nil {
		slog.Error("Failed to fetch secrets from Infisical", "error", err)
		os.Exit(1)
	}

	slog.Info("Successfully injected Infisical secrets into memory!", "count", len(secrets))
}

func main() {
	// Check if APP_ENV is set (e.g., development, production)
	appEnv := os.Getenv("APP_ENV")
	envFile := ".env"
	if appEnv != "" {
		envFile = ".env." + appEnv
	}

	logger.InitLogger(appEnv)

	// Fetch remote secrets before local .env parsing
	loadInfisicalSecrets()

	// If PORT is already set by systemd, skip loading the .env file so we don't accidentally override it
	if os.Getenv("PORT") == "" {
		err := godotenv.Load(envFile)
		if err != nil && appEnv != "" {
			// If .env.development fails, fallback to standard .env
			slog.Error("Could not load %s, falling back to .env", "envFile", envFile)
			godotenv.Load(".env")
		} else if err != nil {
			slog.Warn("No .env file found. Falling back to system environment variables.")
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
			slog.Error("sentry could not be initialized: ", "error", err)
		}
		defer sentry.Flush(2 * time.Second)
		slog.Info("Sentry initialized successfully.")
	}

	app.Run()
}
