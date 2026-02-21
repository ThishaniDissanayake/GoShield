package main

import (
	"log"
	"os"
	"strconv"

	"github.com/ThishaniDissanayake/GoShield/go-rate-limiter/internal/config"
	"github.com/ThishaniDissanayake/GoShield/go-rate-limiter/internal/gateway"
	"github.com/ThishaniDissanayake/GoShield/go-rate-limiter/internal/handlers"
	"github.com/ThishaniDissanayake/GoShield/go-rate-limiter/internal/middleware"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func main() {
	// Load .env (ignore error – env vars may come from Docker/OS)
	godotenv.Load()

	// ── Upstream URL (required) ──────────────────────────────────
	upstreamURL := os.Getenv("UPSTREAM_URL")
	if upstreamURL == "" {
		log.Fatal("❌ UPSTREAM_URL environment variable is required in gateway mode")
	}

	// ── Rate-limit settings ──────────────────────────────────────
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

	mode := os.Getenv("RATE_LIMIT_MODE") // "sliding" (default) or "fixed"

	// ── Redis ────────────────────────────────────────────────────
	config.ConnectRedis()

	// ── Reverse proxy ────────────────────────────────────────────
	proxy := gateway.NewReverseProxy(upstreamURL)

	// ── Gin router ───────────────────────────────────────────────
	r := gin.Default()

	// Health endpoint – no rate limiting, not forwarded upstream.
	r.GET("/health", handlers.HealthCheck)

	// All other routes: rate-limit first, then forward to upstream.
	// NoRoute catches all requests that don't match registered routes.
	r.NoRoute(
		middleware.RateLimiter(rateLimit, windowSeconds, mode),
		gateway.ProxyHandler(proxy),
	)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("🚀 GoShield gateway listening on :%s → %s", port, upstreamURL)
	r.Run(":" + port)
}
