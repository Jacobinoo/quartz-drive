package main

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"
)

// compose runs a `docker compose -f <file> <args...>` command,
// streaming stdout/stderr directly to the terminal.
func compose(ctx context.Context, composeFile, envFile string, args ...string) error {
	cmdArgs := []string{"compose", "-f", composeFile}
	if envFile != "" {
		cmdArgs = append(cmdArgs, "--env-file", envFile)
	}
	cmdArgs = append(cmdArgs, args...)
	cmd := exec.CommandContext(ctx, "docker", cmdArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker compose %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

// dockerAutoLogin attempts to log into Docker Hub if DOCKERHUB_USERNAME and DOCKERHUB_TOKEN are set in the environment.
func dockerAutoLogin(ctx context.Context, log *slog.Logger) {
	username := os.Getenv("DOCKERHUB_USERNAME")
	token := os.Getenv("DOCKERHUB_TOKEN")

	if username == "" || token == "" {
		return // Silently skip if no credentials provided
	}

	log.Info("Found Docker Hub credentials, attempting automatic login...")

	cmd := exec.CommandContext(ctx, "docker", "login", "--username", username, "--password-stdin")
	cmd.Stdin = strings.NewReader(token)

	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Warn("Failed to automatically log into Docker Hub", "error", err, "output", strings.TrimSpace(string(out)))
	} else {
		log.Info("Successfully logged into Docker Hub")
	}
}

// listServiceContainerIDs returns the full container IDs currently
// running for a given compose service. Returns nil (not an error)
// if no containers are running yet.
func listServiceContainerIDs(ctx context.Context, composeFile, envFile, service string) ([]string, error) {
	cmdArgs := []string{"compose", "-f", composeFile}
	if envFile != "" {
		cmdArgs = append(cmdArgs, "--env-file", envFile)
	}
	cmdArgs = append(cmdArgs, "ps", "-q", service)

	cmd := exec.CommandContext(ctx, "docker", cmdArgs...)
	out, err := cmd.Output()
	if err != nil {
		// docker compose ps returns exit 1 when no containers exist — treat as empty
		return nil, nil
	}
	raw := strings.TrimSpace(string(out))
	if raw == "" {
		return nil, nil
	}
	return strings.Fields(raw), nil
}

// waitForHealth polls a container's health/run state until it is
// healthy, becomes unhealthy/exited, or the timeout is reached.
func waitForHealth(ctx context.Context, containerID string, timeout time.Duration, log *slog.Logger) error {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	attempt := 0
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled: %w", ctx.Err())
		case <-ticker.C:
			if time.Now().After(deadline) {
				return fmt.Errorf("timed out after %s waiting for container to become healthy", timeout)
			}

			attempt++
			status, err := containerHealthStatus(ctx, containerID)
			if err != nil {
				// Transient inspect error — keep polling
				log.Warn("Health check inspection error (retrying)", "attempt", attempt, "error", err)
				continue
			}

			log.Info("Health check poll",
				"attempt", attempt,
				"status", status,
				"id", shortID(containerID),
			)

			switch status {
			case "healthy":
				return nil

			case "running-no-healthcheck":
				// Container has no HEALTHCHECK defined but is running — treat as healthy
				log.Warn("Container has no HEALTHCHECK defined — treating running state as healthy")
				return nil

			case "unhealthy":
				return fmt.Errorf("container became unhealthy:\n%s",
					containerTailLogs(containerID, 50))

			case "exited", "stopped", "dead":
				return fmt.Errorf("container exited unexpectedly (status=%s):\n%s",
					status, containerTailLogs(containerID, 50))

			default:
				// "starting" — keep waiting
			}
		}
	}
}

// containerHealthStatus returns a normalized status string for a container.
// Handles containers with and without a HEALTHCHECK directive.
//
// Return values:
//   - "healthy" / "unhealthy" / "starting"  — container has a HEALTHCHECK
//   - "running-no-healthcheck"               — running, no HEALTHCHECK defined
//   - "exited" / "stopped" / "dead"         — container is not running
func containerHealthStatus(ctx context.Context, containerID string) (string, error) {
	// The template distinguishes "has health" vs "no health" at the Docker API level
	const tmpl = `{{if .State.Health}}{{.State.Health.Status}}{{else}}{{if .State.Running}}running-no-healthcheck{{else}}{{.State.Status}}{{end}}{{end}}`

	cmd := exec.CommandContext(ctx, "docker", "inspect", "--format", tmpl, containerID)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("docker inspect %s: %w", shortID(containerID), err)
	}
	return strings.TrimSpace(string(out)), nil
}

// containerTailLogs returns the last N lines of a container's combined stdout+stderr.
// Used to surface failure context during rollback.
func containerTailLogs(containerID string, lines int) string {
	cmd := exec.Command("docker", "logs", "--tail", fmt.Sprintf("%d", lines), containerID)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	_ = cmd.Run() // best-effort — ignore error (container may already be gone)
	return buf.String()
}

// stopContainer sends SIGTERM and waits up to 30s before Docker sends SIGKILL.
func stopContainer(ctx context.Context, containerID string) error {
	cmd := exec.CommandContext(ctx, "docker", "stop", "--time", "30", containerID)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker stop %s: %w\n%s", shortID(containerID), err, out)
	}
	return nil
}

// removeContainer force-removes a container regardless of its state.
func removeContainer(ctx context.Context, containerID string) error {
	cmd := exec.CommandContext(ctx, "docker", "rm", "-f", containerID)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker rm %s: %w\n%s", shortID(containerID), err, out)
	}
	return nil
}

// shortID returns the first 12 chars of a container ID (Docker's default display length).
func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

// shortIDs maps shortID over a slice.
func shortIDs(ids []string) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = shortID(id)
	}
	return out
}
