package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
)

type discordEmbed struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Color       int    `json:"color"`
}

type discordPayload struct {
	Embeds []discordEmbed `json:"embeds"`
}

// notifyDiscord sends a formatted webhook alert to Discord
func notifyDiscord(ctx context.Context, webhookURL, repo, tag string, deployErr error, log *slog.Logger) {
	if webhookURL == "" {
		return
	}

	embed := discordEmbed{
		Title:       "✅ Deployment Succeeded",
		Description: "Successfully deployed **" + repo + ":" + tag + "**",
		Color:       3066993, // Green
	}

	if deployErr != nil {
		embed.Title = "🚨 Deployment Failed"

		// Truncate the error message if it exceeds Discord's 4096 character limit
		errMsg := deployErr.Error()
		if len(errMsg) > 2000 {
			errMsg = errMsg[len(errMsg)-2000:]
		}

		embed.Description = "Failed to deploy **" + repo + ":" + tag + "**\n\n```text\n" + errMsg + "\n```"
		embed.Color = 15158332 // Red
	}

	payload := discordPayload{
		Embeds: []discordEmbed{embed},
	}

	b, err := json.Marshal(payload)
	if err != nil {
		log.Error("Failed to marshal discord payload", "error", err)
		return
	}

	// Set a 5-second timeout for the notification request
	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, "POST", webhookURL, bytes.NewBuffer(b))
	if err != nil {
		log.Error("Failed to create discord request", "error", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Error("Failed to send discord webhook", "error", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		log.Error("Discord webhook returned error status", "status", resp.Status)
	}
}
