package database

import (
	"context"
	"log"
	"quartz/config"
	"time"

	"github.com/redis/go-redis/v9"
)

func NewRedis() *redis.Client {
	for i := 1; i <= 3; i++ {
		log.Printf("redis is connecting... (attempt %d)", i)
		opts, err := redis.ParseURL(config.Cfg.KV.URL)
		if err != nil {
			log.Fatalf("Invalid Redis URL: %v", err)
		}
		client := redis.NewClient(opts)

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_, err = client.Ping(ctx).Result()
		cancel()

		if err == nil {
			log.Println("redis connected")
			return client
		}

		if i < 3 {
			log.Printf("redis connection failed: %v, retrying in 2 seconds...", err)
			time.Sleep(2 * time.Second)
		}
	}
	log.Fatal("failed to connect to redis after 3 attempts")
	return nil
}
