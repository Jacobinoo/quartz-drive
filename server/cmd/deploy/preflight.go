package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"syscall"
)

// runPreflightChecks validates the environment before touching anything.
// Every check must pass or the deployment is aborted before any side effects occur.
func runPreflightChecks(ctx context.Context, cfg *Config) error {
	slog.Info("Running pre-flight checks...")

	checks := []struct {
		name string
		fn   func() error
	}{
		{"compose file exists", func() error { return checkComposeFile(cfg.ComposeFile) }},
		{"docker daemon reachable", func() error { return checkDockerDaemon(ctx) }},
		{"required env vars", checkRequiredEnvVars},
		{"disk space", checkDiskSpace},
	}

	for _, c := range checks {
		if err := c.fn(); err != nil {
			return fmt.Errorf("check %q: %w", c.name, err)
		}
		slog.Info("Pre-flight check passed", "check", c.name)
	}

	slog.Info("All pre-flight checks passed")
	return nil
}

// checkComposeFile verifies the Docker Compose file exists on disk.
func checkComposeFile(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("file not found: %s", path)
	}
	return nil
}

// checkDockerDaemon verifies the Docker daemon is running and accessible.
func checkDockerDaemon(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "docker", "info", "--format", "{{.ServerVersion}}")
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("docker daemon is not reachable — is Docker running? (%w)", err)
	}
	slog.Debug("Docker daemon version", "version", string(out))
	return nil
}

// checkRequiredEnvVars ensures all mandatory env vars are set before deployment.
// These are the vars your docker-compose.prod.yml depends on.
var requiredEnvVars = []string{
	"DOMAIN_NAME",
	"ACME_EMAIL",
	"DOCKERHUB_USERNAME",
}

func checkRequiredEnvVars() error {
	for _, env := range requiredEnvVars {
		if os.Getenv(env) == "" {
			return fmt.Errorf("environment variable %q is not set (did you source .env.production?)", env)
		}
	}
	return nil
}

// checkDiskSpace ensures at least 1 GB is free in the working directory.
// Pulling a Docker image and running two containers simultaneously
// can require several hundred MB of temporary disk space.
func checkDiskSpace() error {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(".", &stat); err != nil {
		// Non-fatal: can't check on this platform / FS type
		slog.Warn("Could not check disk space", "error", err)
		return nil
	}

	freeGB := float64(stat.Bavail*uint64(stat.Bsize)) / 1e9
	slog.Info("Disk space available", "free_gb", fmt.Sprintf("%.2f", freeGB))

	if freeGB < 1.0 {
		return fmt.Errorf("only %.2f GB free — need at least 1 GB for a safe deployment", freeGB)
	}
	return nil
}
