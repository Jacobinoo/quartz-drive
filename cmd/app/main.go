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
		log.Fatalf("unable to load .env file: %e", err)
	}

	config.Cfg.Init()
	app.Run()
}
