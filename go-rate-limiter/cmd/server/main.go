package main

import (
	"os"
	"strconv"

	"github.com/ThishaniDissanayake/GoShield/go-rate-limiter/internal/config"
	"github.com/ThishaniDissanayake/GoShield/go-rate-limiter/internal/handlers"
	"github.com/ThishaniDissanayake/GoShield/go-rate-limiter/internal/middleware"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func main() {
	// Load .env
	godotenv.Load()

	rateLimit, windowSeconds := 100, 60
	if v := os.Getenv("RATE_LIMIT"); v != "" {
		if x, err := strconv.Atoi(v); err == nil {
			rateLimit = x
		}
	}
	if v := os.Getenv("WINDOW_SECONDS"); v != "" {
		if x, err := strconv.Atoi(v); err == nil {
			windowSeconds = x
		}
	}

	config.ConnectRedis()

	r := gin.Default()
	r.Use(middleware.RateLimiter(rateLimit, windowSeconds))

	r.GET("/health", handlers.HealthCheck)
	r.Run(":8080")
}
