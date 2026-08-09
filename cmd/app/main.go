package main

import (
	"log"
	"quartz/config"
	"quartz/internal/app"

	"github.com/joho/godotenv"
)

func main() {
	err := godotenv.Load(".env")
	if err != nil {
		log.Printf("No .env file found. Falling back to system environment variables.")
	}

	config.Cfg.Init()
	app.Run()
}
