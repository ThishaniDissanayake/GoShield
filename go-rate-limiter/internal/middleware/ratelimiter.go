package middleware

import (
	"net/http"
	"time"

	"github.com/ThishaniDissanayake/GoShield/go-rate-limiter/internal/config"
	"github.com/gin-gonic/gin"
)

const LIMIT = 100
const WINDOW = time.Minute

func RateLimiter() gin.HandlerFunc {
	return func(c *gin.Context) {

		ip := c.ClientIP()
		key := "rate:" + ip

		count, err := config.RedisClient.Incr(config.Ctx, key).Result()
		if err != nil {
			c.JSON(500, gin.H{"error": "Redis error"})
			c.Abort()
			return
		}

		if count == 1 {
			config.RedisClient.Expire(config.Ctx, key, WINDOW)
		}

		if count > LIMIT {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error": "Too many requests",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}
