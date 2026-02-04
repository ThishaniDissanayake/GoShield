package main

import (
	"go-rate-limiter/internal/config"
	"go-rate-limiter/internal/handlers"
	"go-rate-limiter/internal/middleware"

	"github.com/gin-gonic/gin"
)

func main() {

	config.ConnectRedis()

	r := gin.Default()

	r.Use(middleware.RateLimiter())

	r.GET("/health", handlers.HealthCheck)

	r.Run(":8080")
}
