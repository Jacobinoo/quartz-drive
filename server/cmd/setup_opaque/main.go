package main

import (
	"encoding/base64"
	"fmt"
	"log"
	"quartz/internal/bindings"
)

func main() {
	setupBytes, err := bindings.GenerateServerSetup()
	if err != nil {
		log.Fatalf("Failed to generate server setup: %v", err)
	}

	encodedSetup := base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(setupBytes)

	fmt.Println("\n=======================================================")
	fmt.Println("Generated OPAQUE_SERVER_SETUP successfully!")
	fmt.Println("Add the following line to your backend .env file:")
	fmt.Println("=======================================================")
	fmt.Printf("OPAQUE_SERVER_SETUP=%s\n\n", encodedSetup)
	fmt.Println("=======================================================")
	fmt.Println("⚠️  KEEP THIS SECRET! If you lose this value, all user passwords will be invalidated.")
	fmt.Println("=======================================================")
}
