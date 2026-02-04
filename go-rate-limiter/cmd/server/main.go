package main

import (
	"github.com/ThishaniDissanayake/GoShield/go-rate-limiter/internal/config"
	"github.com/ThishaniDissanayake/GoShield/go-rate-limiter/internal/handlers"
	"github.com/ThishaniDissanayake/GoShield/go-rate-limiter/internal/middleware"

	"github.com/gin-gonic/gin"
)

func main() {

	config.ConnectRedis()

	r := gin.Default()

	r.Use(middleware.RateLimiter())

	r.GET("/health", handlers.HealthCheck)

	r.Run(":8080")
}
