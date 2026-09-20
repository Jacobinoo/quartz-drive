package main

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Deployer orchestrates a zero-downtime blue/green deployment
// using Docker Compose on a single server.
type Deployer struct {
	cfg *Config
	log *slog.Logger
}

func (d *Deployer) Run(ctx context.Context) error {
	start := time.Now()

	d.log.Info("Starting deployment",
		"service", d.cfg.Service,
		"compose_file", d.cfg.ComposeFile,
		"health_timeout", d.cfg.HealthTimeout,
		"qdeploy_version", version,
	)

	// 1. Load the env file so all subsequent steps (including docker compose)
	//    have access to variables like DOMAIN_NAME, ACME_EMAIL, etc.
	//    Resolve the env file path relative to the compose file's directory,
	//    since both files live together on the server.
	if err := d.loadEnv(); err != nil {
		return err
	}

	// 1b. Set DEPLOY_TAG so docker-compose.prod.yml uses the correct image tag.
	//     The compose file uses ${DEPLOY_TAG:-latest}, so if -tag is empty we
	//     leave the env var unset and Compose falls back to :latest automatically.
	deployTag := d.cfg.DeployTag
	if deployTag == "" {
		deployTag = "latest"
	}
	_ = os.Setenv("DEPLOY_TAG", deployTag)
	d.log.Info("Deploy tag set", "tag", deployTag)

	// 2. Pre-flight: verify the environment is safe to deploy into
	if err := runPreflightChecks(ctx, d.cfg); err != nil {
		return fmt.Errorf("pre-flight failed: %w", err)
	}

	if d.cfg.DryRun {
		d.log.Info("DRY RUN complete — no changes made")
		return nil
	}

	// 3. Exclusive lock: prevent concurrent deployments
	lock, err := acquireLock(lockFilePath)
	if err != nil {
		return err
	}
	defer lock.release()

	// 3. Snapshot the currently running container IDs *before* we touch anything.
	//    This is what lets us distinguish "old" vs "new" after the scale-up.
	oldIDs, err := listServiceContainerIDs(ctx, d.cfg.ComposeFile, d.cfg.EnvFile, d.cfg.Service)
	if err != nil {
		return fmt.Errorf("listing existing containers: %w", err)
	}
	d.log.Info("Existing containers", "count", len(oldIDs), "ids", shortIDs(oldIDs))

	// 4. Ensure supporting infrastructure (Traefik, Valkey) is running
	d.log.Info("Ensuring infrastructure services are up...")
	if err := compose(ctx, d.cfg.ComposeFile, d.cfg.EnvFile, "up", "-d", "traefik", "valkey"); err != nil {
		return fmt.Errorf("starting infrastructure: %w", err)
	}

	// 5. Pull the latest image *before* starting a new container
	d.log.Info("Pulling latest image...")
	if err := compose(ctx, d.cfg.ComposeFile, d.cfg.EnvFile, "pull", d.cfg.Service); err != nil {
		return fmt.Errorf("pulling image: %w", err)
	}

	// 5b. Ensure we only have exactly 1 old container before scaling up.
	//     If a previous deploy crashed or the user manually scaled up,
	//     we gracefully kill the extras so we can cleanly scale to exactly 2.
	if len(oldIDs) > 1 {
		d.log.Warn("Found multiple old containers, cleaning up extras...", "count", len(oldIDs)-1)
		for _, id := range oldIDs[1:] {
			_ = stopContainer(ctx, id)
			_ = removeContainer(ctx, id)
		}
		oldIDs = oldIDs[:1]
	}

	// 6. Scale up to exactly oldIDs + 1 — creates the pendulum effect.
	//    If index 1 is running, Compose creates index 2.
	//    If index 2 is running, Compose creates index 1!
	//    If nothing is running, it creates exactly 1.
	//    --no-recreate ensures existing containers are NOT touched.
	targetScale := len(oldIDs) + 1
	d.log.Info("Starting new container alongside old one...", "pendulum_scale", targetScale)
	scaleUp := fmt.Sprintf("%s=%d", d.cfg.Service, targetScale)
	if err := compose(ctx, d.cfg.ComposeFile, d.cfg.EnvFile, "up", "-d",
		"--scale", scaleUp,
		"--no-recreate",
		d.cfg.Service,
	); err != nil {
		return fmt.Errorf("scaling up: %w", err)
	}

	// Brief pause so Docker registers the new container before we query it
	time.Sleep(2 * time.Second)

	// 7. Find which container ID is new (not in our pre-scale snapshot)
	allIDs, err := listServiceContainerIDs(ctx, d.cfg.ComposeFile, d.cfg.EnvFile, d.cfg.Service)
	if err != nil {
		return fmt.Errorf("listing containers after scale-up: %w", err)
	}

	newID := findNew(allIDs, oldIDs)
	if newID == "" {
		d.log.Error("Could not identify new container",
			"all_ids", shortIDs(allIDs),
			"old_ids", shortIDs(oldIDs),
		)
		return fmt.Errorf("no new container found — scale-up may have recycled the existing container")
	}
	d.log.Info("New container identified", "id", shortID(newID))

	// 8. Wait for the new container to pass its health check
	d.log.Info("Waiting for health check...", "timeout", d.cfg.HealthTimeout)
	if err := waitForHealth(ctx, newID, d.cfg.HealthTimeout, d.log); err != nil {
		d.log.Error("Health check failed — initiating rollback", "error", err)
		return d.rollback(ctx, newID, err)
	}

	d.log.Info("New container is healthy", "id", shortID(newID))

	// 9. Stop and remove old containers (graceful stop, then rm)
	for _, id := range oldIDs {
		d.log.Info("Stopping old container", "id", shortID(id))
		if err := stopContainer(ctx, id); err != nil {
			// Non-fatal: it may have already exited
			d.log.Warn("Could not stop old container", "error", err, "id", shortID(id))
		}
		if err := removeContainer(ctx, id); err != nil {
			d.log.Warn("Could not remove old container", "error", err, "id", shortID(id))
		}
	}

	// We purposely DO NOT scale back down to 1 here.
	// If we scale back to 1, Docker Compose V2 enforces index <= scale,
	// which means it would instantly assassinate our brand new container
	// (if it has index 2) and start index 1 in the background!
	// Leaving the compose state at scale=2 allows the pendulum to work perfectly.

	d.log.Info("Deployment complete",
		"duration", time.Since(start).Round(time.Millisecond),
		"container", shortID(newID),
	)
	return nil
}

// rollback removes the unhealthy new container and restores scale to 1,
// leaving the old container untouched and still serving traffic.
func (d *Deployer) rollback(ctx context.Context, newID string, reason error) error {
	d.log.Info("Rollback: removing unhealthy container...", "id", shortID(newID))

	if err := removeContainer(ctx, newID); err != nil {
		d.log.Error("Rollback: failed to remove unhealthy container",
			"error", err,
			"id", shortID(newID),
		)
	} else {
		d.log.Info("Rollback: unhealthy container removed", "id", shortID(newID))
	}

	// Rollback: we don't scale reset here either, to avoid the same issue.
	// Since we removed the unhealthy new container, the old one remains at its index.

	d.log.Info("Rollback complete — old container is still serving traffic")
	return fmt.Errorf("deployment failed (rolled back): %w", reason)
}

// findNew returns the first ID in `all` that doesn't appear in `old`.
func findNew(all, old []string) string {
	oldSet := make(map[string]bool, len(old))
	for _, id := range old {
		oldSet[id] = true
	}
	for _, id := range all {
		if !oldSet[id] {
			return id
		}
	}
	return ""
}

// loadEnvFile reads a .env file and sets each key=value pair as an
// environment variable for this process and all child processes it spawns
// (docker compose inherits the environment automatically).
//
// Rules:
//   - Lines starting with # are comments and are skipped
//   - Blank lines are skipped
//   - Existing env vars are NOT overridden (shell-set vars take precedence)
//   - Values may optionally be wrapped in single or double quotes
func loadEnvFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	lineNum := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip blank lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Split on the first `=` only — values may contain `=` themselves
		idx := strings.IndexByte(line, '=')
		if idx < 0 {
			// Not a valid key=value pair — skip silently
			continue
		}

		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])

		// Strip surrounding quotes (single or double) if present
		if len(val) >= 2 {
			if (val[0] == '"' && val[len(val)-1] == '"') ||
				(val[0] == '\'' && val[len(val)-1] == '\'') {
				val = val[1 : len(val)-1]
			}
		}

		// Don't override a var that's already set in the environment
		if os.Getenv(key) != "" {
			continue
		}

		if err := os.Setenv(key, val); err != nil {
			return fmt.Errorf("line %d: setenv %q: %w", lineNum, key, err)
		}
	}

	return scanner.Err()
}

func (d *Deployer) loadEnv() error {
	if d.cfg.EnvFile == "" {
		return nil
	}
	envPath := d.cfg.EnvFile
	if !filepath.IsAbs(envPath) {
		envPath = filepath.Join(filepath.Dir(d.cfg.ComposeFile), envPath)
	}
	if err := loadEnvFile(envPath); err != nil {
		return fmt.Errorf("loading env file %q: %w", envPath, err)
	}
	d.log.Info("Env file loaded", "path", envPath)
	return nil
}
