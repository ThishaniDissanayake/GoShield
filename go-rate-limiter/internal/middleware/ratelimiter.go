package middleware

import (
	"log"
	"net/http"
	"time"

	"github.com/ThishaniDissanayake/GoShield/go-rate-limiter/internal/config"
	"github.com/ThishaniDissanayake/GoShield/go-rate-limiter/internal/ratelimiter"
	"github.com/gin-gonic/gin"
)

// RateLimiter returns a Gin middleware that enforces per-IP rate limiting.
// mode should be "sliding" or "fixed". Any other value defaults to "sliding".
func RateLimiter(limit int, windowSeconds int, mode string) gin.HandlerFunc {
	if mode == "" {
		mode = "sliding"
	}
	log.Printf("⚙️  Rate-limit mode: %s  |  limit: %d  |  window: %ds", mode, limit, windowSeconds)

	if mode == "fixed" {
		return fixedWindowLimiter(limit, windowSeconds)
	}
	return slidingWindowLimiter(limit, windowSeconds)
}

// ── Fixed-window (legacy) ─────────────────────────────────────────────

func fixedWindowLimiter(limit int, windowSeconds int) gin.HandlerFunc {
	window := time.Duration(windowSeconds) * time.Second

	return func(c *gin.Context) {
		ip := c.ClientIP()
		key := "rate:fixed:" + ip

		count, err := config.RDB.Incr(config.Ctx, key).Result()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Redis error"})
			c.Abort()
			return
		}

		if count == 1 {
			config.RDB.Expire(config.Ctx, key, window)
		}

		if count > int64(limit) {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":          "Too many requests",
				"limit":          limit,
				"window_seconds": windowSeconds,
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// ── Sliding-window (default) ──────────────────────────────────────────

func slidingWindowLimiter(limit int, windowSeconds int) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()

		result, err := ratelimiter.CheckSlidingWindow(
			config.Ctx, config.RDB, ip, limit, windowSeconds,
		)
		if err != nil {
			log.Printf("❌ Sliding-window error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Redis error"})
			c.Abort()
			return
		}

		if !result.Allowed {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":          "Too many requests",
				"limit":          result.Limit,
				"window_seconds": result.WindowSec,
			})
			c.Abort()
			return
		}

		c.Next()
	}
}
