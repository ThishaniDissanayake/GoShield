package middleware

import (
	"log"
	"net/http"

	"github.com/ThishaniDissanayake/GoShield/go-rate-limiter/internal/config"
	"github.com/ThishaniDissanayake/GoShield/go-rate-limiter/internal/ratelimiter"
	"github.com/gin-gonic/gin"
)

// ────────────────────────────────────────────────────────────────────────
// GoShield Rate-Limiter Middleware
// ────────────────────────────────────────────────────────────────────────
//
// This middleware intercepts every request and decides whether to allow
// or block it based on per-IP request counts stored in Redis.
//
// Two algorithms are available, both sharing the same guarantees:
//
//   ┌──────────────────────────────────────────────────────────────────┐
//   │  O(1) TIME COMPLEXITY                                           │
//   │                                                                  │
//   │  Fixed window:   INCR + EXPIRE = two O(1) Redis commands.       │
//   │  Sliding window: ZSET ops are O(log N), but N ≤ limit, so the  │
//   │                  cost is bounded and effectively constant.       │
//   │                                                                  │
//   │  Whether 1 or 100,000 users send requests, each individual      │
//   │  request performs the same small number of operations.           │
//   │  → Predictable latency. High throughput. No degradation.        │
//   └──────────────────────────────────────────────────────────────────┘
//
//   ┌──────────────────────────────────────────────────────────────────┐
//   │  ZERO RACE CONDITIONS                                           │
//   │                                                                  │
//   │  Both algorithms execute all Redis operations inside a single   │
//   │  atomic Lua script. Redis is single-threaded; it runs each      │
//   │  script to completion before processing the next command.        │
//   │                                                                  │
//   │  If 1,000 requests arrive at the same instant:                  │
//   │    • Redis queues and processes them one-by-one.                 │
//   │    • Every caller sees a unique, correct counter value.          │
//   │    • No read-modify-write races. No lost updates. No phantom    │
//   │      reads.                                                      │
//   │                                                                  │
//   │  This is fundamentally different from a naive pattern like:     │
//   │    GET → add 1 locally → SET   (BROKEN under concurrency)       │
//   │                                                                  │
//   │  GoShield uses:                                                  │
//   │    EVAL Lua { INCR / ZADD… }   (SAFE under concurrency)        │
//   └──────────────────────────────────────────────────────────────────┘
//
//   ┌──────────────────────────────────────────────────────────────────┐
//   │  PRODUCTION SUITABILITY                                          │
//   │                                                                  │
//   │  • Horizontally scalable: multiple GoShield instances share     │
//   │    one Redis and always agree on counts.                         │
//   │  • Stateless application: GoShield itself holds no counters;    │
//   │    it can be restarted or scaled at will.                        │
//   │  • Memory-safe: every Redis key auto-expires via TTL.            │
//   └──────────────────────────────────────────────────────────────────┘
// ────────────────────────────────────────────────────────────────────────

// RateLimiter returns a Gin middleware that enforces per-IP rate limiting.
//
// Parameters:
//   - limit:         max requests allowed per window (e.g. 100)
//   - windowSeconds: window duration in seconds (e.g. 60)
//   - mode:          "fixed" for O(1) fixed-window counter,
//                    "sliding" (default) for sliding-window ZSET.
//
// Both modes guarantee O(1) effective time complexity and zero race
// conditions via atomic Redis Lua scripts.
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

// ── Fixed-window limiter ──────────────────────────────────────────────
//
// Uses the atomic Lua script in ratelimiter.CheckFixedWindow which
// performs INCR + conditional EXPIRE in a single uninterruptible call.
//
// Time complexity:  O(1) per request — guaranteed.
// Race conditions:  Zero — guaranteed by atomic Lua execution.
func fixedWindowLimiter(limit int, windowSeconds int) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()

		result, err := ratelimiter.CheckFixedWindow(
			config.Ctx, config.RDB, ip, limit, windowSeconds,
		)
		if err != nil {
			log.Printf("❌ Fixed-window error: %v", err)
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

// ── Sliding-window limiter ────────────────────────────────────────────
//
// Uses the atomic Lua script in ratelimiter.CheckSlidingWindow which
// performs ZREMRANGEBYSCORE + ZADD + ZCARD + EXPIRE in a single call.
//
// Time complexity:  Amortised O(1) — ZSET size bounded by limit.
// Race conditions:  Zero — guaranteed by atomic Lua execution.
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
