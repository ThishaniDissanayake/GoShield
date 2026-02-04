package config

import (
	"context"
	"log"
	"os"

	"github.com/redis/go-redis/v9"
)

var Ctx = context.Background()
var RDB *redis.Client

func ConnectRedis() {
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "redis:6379" // docker service name
	}

	RDB = redis.NewClient(&redis.Options{
		Addr: addr,
	})

	_, err := RDB.Ping(Ctx).Result()
	if err != nil {
		log.Fatalf("❌ Redis connection failed: %v", err)
	}

	log.Println("✅ Connected to Redis")
}
