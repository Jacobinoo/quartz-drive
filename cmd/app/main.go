package main

import (
	"log"
	"os"
	"quartz/config"
	"quartz/internal/app"

	"github.com/joho/godotenv"
)

func main() {
	// If PORT is already set by systemd, skip loading the .env file so we don't accidentally override it
	if os.Getenv("PORT") == "" {
		err := godotenv.Load(".env")
		if err != nil {
			log.Printf("No .env file found. Falling back to system environment variables.")
		}
	}

	config.Cfg.Init()
	app.Run()
}
