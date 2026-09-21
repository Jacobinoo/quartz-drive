package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func StartMonitor(ctx context.Context, cfg *Config, log *slog.Logger) {
	if cfg.WebhookURL == "" {
		webhook := os.Getenv("WEBHOOK_URL")
		if webhook == "" {
			log.Info("WEBHOOK_URL not set, infrastructure monitoring alerts disabled.")
			return
		}
		cfg.WebhookURL = webhook
	}

	log.Info("Starting infrastructure and crash monitor...")

	// 1. Docker Crash Monitor
	go monitorDockerCrashes(ctx, cfg, log)

	// 2. Resource Monitor (CPU/RAM)
	go monitorResources(ctx, cfg, log)
}

func monitorDockerCrashes(ctx context.Context, cfg *Config, log *slog.Logger) {
	// docker events --filter 'event=die' --format '{{json .}}'
	cmd := exec.CommandContext(ctx, "docker", "events", "--filter", "event=die", "--format", "{{json .}}")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Error("Failed to attach to docker events", "error", err)
		return
	}

	if err := cmd.Start(); err != nil {
		log.Error("Failed to start docker events monitor", "error", err)
		return
	}

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		var event struct {
			Actor struct {
				Attributes struct {
					Name     string `json:"name"`
					ExitCode string `json:"exitCode"`
				} `json:"Attributes"`
			} `json:"Actor"`
		}

		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}

		exitCode := event.Actor.Attributes.ExitCode
		name := event.Actor.Attributes.Name

		// If exit code is not 0, it's a crash
		if exitCode != "0" && exitCode != "" {
			log.Warn("Container crashed!", "name", name, "exitCode", exitCode)

			// Check if it was OOM killed
			isOOM := false
			inspectCmd := exec.CommandContext(ctx, "docker", "inspect", name, "--format", "{{.State.OOMKilled}}")
			out, _ := inspectCmd.Output()
			if strings.TrimSpace(string(out)) == "true" {
				isOOM = true
			}

			reason := "Container crashed with exit code " + exitCode
			if isOOM {
				reason = "Container was OOMKilled (Out of Memory)!"
			}

			errObj := fmt.Errorf("%s", reason)
			notifyDiscord(ctx, cfg.WebhookURL, "Container Crash: "+name, "latest", errObj, log)
		}
	}
}

func monitorResources(ctx context.Context, cfg *Config, log *slog.Logger) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	highMemCount := 0
	highLoadCount := 0

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// 1. Check Memory using /proc/meminfo
			memTotal, memAvail := getMemoryStats()
			if memTotal > 0 {
				usedPercent := 100.0 * (1.0 - float64(memAvail)/float64(memTotal))
				if usedPercent > 80.0 {
					highMemCount++
					if highMemCount >= 2 {
						log.Warn("High memory usage detected!", "percent", usedPercent)
						notifyDiscord(ctx, cfg.WebhookURL, "Infrastructure Warning", "Resource Alert", fmt.Errorf("High Memory Usage: %.1f%% for 2 minutes", usedPercent), log)
						highMemCount = 0 // Reset to avoid spam
					}
				} else {
					highMemCount = 0
				}
			}

			// 2. Check Load Average using /proc/loadavg
			loadAvg, err := getLoadAvg()
			if err == nil {
				if loadAvg > 1.0 {
					highLoadCount++
					if highLoadCount >= 2 {
						log.Warn("High CPU load detected!", "load1m", loadAvg)
						notifyDiscord(ctx, cfg.WebhookURL, "Infrastructure Warning", "Resource Alert", fmt.Errorf("High CPU Load Average: %.2f for 2 minutes", loadAvg), log)
						highLoadCount = 0 // Reset to avoid spam
					}
				} else {
					highLoadCount = 0
				}
			}
		}
	}
}

func getLoadAvg() (float64, error) {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(string(data))
	if len(fields) < 1 {
		return 0, fmt.Errorf("invalid format")
	}
	return strconv.ParseFloat(fields[0], 64)
}

func getMemoryStats() (total, available uint64) {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		val, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}

		switch fields[0] {
		case "MemTotal:":
			total = val
		case "MemAvailable:":
			available = val
		}
	}
	return total, available
}
