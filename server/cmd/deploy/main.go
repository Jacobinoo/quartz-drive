package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const (
	version      = "1.0.0"
	lockFilePath = "/tmp/qdeploy.lock"
)

type Config struct {
	ComposeFile   string
	EnvFile       string
	Service       string
	DeployTag     string
	HealthTimeout time.Duration
	DryRun        bool
	PrintVersion  bool

	Watch         bool
	WatchInterval time.Duration
	GitHubRepo    string
}

func main() {
	cfg := &Config{}

	flag.StringVar(&cfg.ComposeFile, "f", "docker-compose.prod.yml", "Docker Compose file to use")
	flag.StringVar(&cfg.EnvFile, "env-file", ".env.production", "Env file to load before deployment (relative to -f location)")
	flag.StringVar(&cfg.Service, "service", "quartz-server", "Service name to deploy")
	flag.StringVar(&cfg.DeployTag, "tag", "", "Docker image tag to deploy (e.g. sha-a3f9c1b, v0.1.0, build-42). Defaults to 'latest' if empty.")
	flag.DurationVar(&cfg.HealthTimeout, "timeout", 90*time.Second, "Max time to wait for new container to become healthy")
	flag.BoolVar(&cfg.DryRun, "dry-run", false, "Preview what would happen without making any changes")
	flag.BoolVar(&cfg.PrintVersion, "version", false, "Print version and exit")

	flag.BoolVar(&cfg.Watch, "watch", false, "Run in background and poll for new GitHub releases")
	flag.DurationVar(&cfg.WatchInterval, "watch-interval", 3*time.Minute, "How often to poll GitHub for new releases")
	flag.StringVar(&cfg.GitHubRepo, "github-repo", "Jacobinoo/quartz-drive", "GitHub repository to poll for latest release")
	flag.Parse()

	if cfg.PrintVersion {
		println("qdeploy", version)
		os.Exit(0)
	}

	// JSON logging — works natively with GCP Cloud Logging
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	slog.SetDefault(log)

	// Graceful shutdown on SIGINT / SIGTERM
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	d := &Deployer{cfg: cfg, log: log}

	if cfg.Watch {
		if err := d.Watch(ctx); err != nil {
			log.Error("Watch mode failed", "error", err)
			os.Exit(1)
		}
	} else {
		if err := d.Run(ctx); err != nil {
			log.Error("Deployment failed", "error", err)
			os.Exit(1)
		}
	}
}
