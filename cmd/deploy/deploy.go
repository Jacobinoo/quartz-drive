package main

import (
	"context"
	"fmt"
	"log/slog"
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

	// 1. Pre-flight: verify the environment is safe to deploy into
	if err := runPreflightChecks(ctx, d.cfg); err != nil {
		return fmt.Errorf("pre-flight failed: %w", err)
	}

	if d.cfg.DryRun {
		d.log.Info("DRY RUN complete — no changes made")
		return nil
	}

	// 2. Exclusive lock: prevent concurrent deployments
	lock, err := acquireLock(lockFilePath)
	if err != nil {
		return err
	}
	defer lock.release()

	// 3. Snapshot the currently running container IDs *before* we touch anything.
	//    This is what lets us distinguish "old" vs "new" after the scale-up.
	oldIDs, err := listServiceContainerIDs(ctx, d.cfg.ComposeFile, d.cfg.Service)
	if err != nil {
		return fmt.Errorf("listing existing containers: %w", err)
	}
	d.log.Info("Existing containers", "count", len(oldIDs), "ids", shortIDs(oldIDs))

	// 4. Ensure supporting infrastructure (Traefik, Valkey) is running
	d.log.Info("Ensuring infrastructure services are up...")
	if err := compose(ctx, d.cfg.ComposeFile, "up", "-d", "traefik", "valkey"); err != nil {
		return fmt.Errorf("starting infrastructure: %w", err)
	}

	// 5. Pull the latest image *before* starting a new container
	d.log.Info("Pulling latest image...")
	if err := compose(ctx, d.cfg.ComposeFile, "pull", d.cfg.Service); err != nil {
		return fmt.Errorf("pulling image: %w", err)
	}

	// 6. Scale up to 2 — starts the new container alongside the old one.
	//    --no-recreate ensures the old container is NOT touched.
	d.log.Info("Starting new container alongside old one...")
	scaleUp := fmt.Sprintf("%s=2", d.cfg.Service)
	if err := compose(ctx, d.cfg.ComposeFile, "up", "-d",
		"--scale", scaleUp,
		"--no-recreate",
		d.cfg.Service,
	); err != nil {
		return fmt.Errorf("scaling up: %w", err)
	}

	// Brief pause so Docker registers the new container before we query it
	time.Sleep(2 * time.Second)

	// 7. Find which container ID is new (not in our pre-scale snapshot)
	allIDs, err := listServiceContainerIDs(ctx, d.cfg.ComposeFile, d.cfg.Service)
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

	// 10. Scale back down to 1 so compose state is consistent
	d.log.Info("Resetting scale to 1...")
	scaleDown := fmt.Sprintf("%s=1", d.cfg.Service)
	if err := compose(ctx, d.cfg.ComposeFile, "up", "-d",
		"--scale", scaleDown,
		"--no-deps",
		d.cfg.Service,
	); err != nil {
		// Non-fatal: the new container is healthy and serving traffic regardless
		d.log.Warn("Scale reset failed (non-fatal — new container is still healthy)", "error", err)
	}

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

	// Reset compose scale state back to 1
	scaleDown := fmt.Sprintf("%s=1", d.cfg.Service)
	if err := compose(ctx, d.cfg.ComposeFile, "up", "-d",
		"--scale", scaleDown,
		"--no-deps",
		d.cfg.Service,
	); err != nil {
		d.log.Error("Rollback: scale reset failed", "error", err)
	}

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
