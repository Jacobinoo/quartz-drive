package discord

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"time"
)

type discordEmbed struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Color       int    `json:"color"`
}

type discordPayload struct {
	Content string         `json:"content,omitempty"`
	Embeds  []discordEmbed `json:"embeds,omitempty"`
}

func Notify(ctx context.Context, title, description string, color int) {
	webhookURL := os.Getenv("WEBHOOK_URL")
	if webhookURL == "" {
		return
	}

	// Truncate description if it exceeds Discord's limit
	if len(description) > 2000 {
		description = description[len(description)-2000:]
	}

	embed := discordEmbed{
		Title:       title,
		Description: description,
		Color:       color,
	}

	payload := discordPayload{
		Embeds: []discordEmbed{embed},
	}

	b, err := json.Marshal(payload)
	if err != nil {
		slog.Error("Failed to marshal discord payload", "error", err)
		return
	}

	go func() {
		reqCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		req, err := http.NewRequestWithContext(reqCtx, "POST", webhookURL, bytes.NewBuffer(b))
		if err != nil {
			slog.Error("Failed to create discord request", "error", err)
			return
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			slog.Error("Failed to send discord webhook", "error", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			slog.Error("Discord webhook returned error status", "status", resp.Status)
		}
	}()
}
