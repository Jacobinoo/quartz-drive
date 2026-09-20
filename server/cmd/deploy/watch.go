package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type githubRelease struct {
	TagName string `json:"tag_name"`
}

func (d *Deployer) Watch(ctx context.Context) error {
	d.log.Info("Starting Watch mode", "repo", d.cfg.GitHubRepo, "interval", d.cfg.WatchInterval)

	ticker := time.NewTicker(d.cfg.WatchInterval)
	defer ticker.Stop()

	// Load environment variables so we have DOCKERHUB_USERNAME and DOCKERHUB_TOKEN
	if err := d.loadEnv(); err != nil {
		d.log.Warn("Failed to load env file in watch mode", "error", err)
	}

	// Attempt automatic docker login
	dockerAutoLogin(ctx, d.log)

	// Initial poll immediately
	d.pollLatestRelease(ctx)

	for {
		select {
		case <-ctx.Done():
			d.log.Info("Watch mode shutting down...")
			return nil
		case <-ticker.C:
			d.pollLatestRelease(ctx)
		}
	}
}

func (d *Deployer) pollLatestRelease(ctx context.Context) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", d.cfg.GitHubRepo)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		d.log.Error("Failed to create request for GitHub releases", "error", err)
		return
	}

	if d.cfg.GitHubToken != "" {
		req.Header.Set("Authorization", "Bearer "+d.cfg.GitHubToken)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		d.log.Error("Failed to fetch latest release", "error", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusNotFound {
			d.log.Debug("No releases found yet")
			return
		}
		d.log.Error("Unexpected status from GitHub API", "status", resp.Status)
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		d.log.Error("Failed to read GitHub response", "error", err)
		return
	}

	var release githubRelease
	if err := json.Unmarshal(body, &release); err != nil {
		d.log.Error("Failed to decode GitHub response", "error", err)
		return
	}

	latestTag := release.TagName
	if latestTag == "" {
		return
	}

	// Read current deployed version
	currentTag := d.readLocalState(".qdeploy_version")

	if latestTag == currentTag {
		// Already up to date
		return
	}

	// Read failed tag
	failedTag := d.readLocalState(".qdeploy_failed")
	if latestTag == failedTag {
		d.log.Info("Skipping tag because it previously failed to deploy. Push a new release to clear.", "tag", latestTag)
		return
	}

	d.log.Info("New release detected!", "current", currentTag, "latest", latestTag)

	// Try to pull the image first to ensure GitHub Actions has finished pushing it.
	// docker-compose.prod.yml uses ${DEPLOY_TAG:-latest}, so we set DEPLOY_TAG here
	// before calling compose.
	_ = os.Setenv("DEPLOY_TAG", latestTag)

	d.log.Info("Attempting to pull image...", "tag", latestTag)
	if err := compose(ctx, d.cfg.ComposeFile, d.cfg.EnvFile, "pull", d.cfg.Service); err != nil {
		d.log.Info("Image not available yet (or pull failed), will retry later.", "error", err)
		return
	}

	d.log.Info("Image pulled successfully. Starting deployment...")

	// Backup original tag and set the new one for the actual run
	oldTag := d.cfg.DeployTag
	d.cfg.DeployTag = latestTag

	if err := d.Run(ctx); err != nil {
		d.log.Error("Deployment failed for new release", "tag", latestTag, "error", err)
		// Write to failed state to prevent infinite crash loop
		d.writeLocalState(".qdeploy_failed", latestTag)
	} else {
		d.log.Info("Deployment succeeded for new release", "tag", latestTag)
		// Write to success state
		d.writeLocalState(".qdeploy_version", latestTag)
		// Clear failed state just in case
		_ = os.Remove(".qdeploy_failed")
	}

	// Restore original
	d.cfg.DeployTag = oldTag
}

func (d *Deployer) readLocalState(filename string) string {
	b, err := os.ReadFile(filename)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func (d *Deployer) writeLocalState(filename, val string) {
	_ = os.WriteFile(filename, []byte(val), 0644)
}
