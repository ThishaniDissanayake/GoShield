package ratelimiter

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// slidingWindowScript is an atomic Lua script that implements the
// sliding-window rate limiting algorithm using a Redis Sorted Set.
//
// Steps executed atomically:
//  1. Remove all entries older than the window.
//  2. Add the current timestamp as both score and member.
//  3. Count remaining entries (= requests inside the window).
//  4. Set a TTL slightly longer than the window to avoid memory leaks.
//  5. Return the count.
var slidingWindowScript = redis.NewScript(`
local key          = KEYS[1]
local now          = tonumber(ARGV[1])
local window       = tonumber(ARGV[2])
local expire_sec   = tonumber(ARGV[3])
local member       = ARGV[4]

-- 1. Remove timestamps older than the window
redis.call("ZREMRANGEBYSCORE", key, 0, now - window)

-- 2. Add this request's timestamp
redis.call("ZADD", key, now, member)

-- 3. Count requests inside the window
local count = redis.call("ZCARD", key)

-- 4. Refresh TTL so the key self-cleans
redis.call("EXPIRE", key, expire_sec)

return count
`)

// SlidingWindowResult holds the outcome of a rate-limit check.
type SlidingWindowResult struct {
	Allowed   bool
	Count     int64
	Limit     int
	WindowSec int
}

// CheckSlidingWindow performs a sliding-window rate-limit check for the
// given identifier (e.g. an IP address).  It returns whether the request
// is allowed and the current request count inside the window.
//
// All Redis operations are executed in a single atomic Lua script so the
// algorithm is safe under concurrent access from multiple GoShield
// instances sharing the same Redis.
func CheckSlidingWindow(ctx context.Context, rdb *redis.Client, identifier string, limit int, windowSeconds int) (*SlidingWindowResult, error) {
	now := time.Now().UnixMilli()                    // millisecond precision
	windowMs := int64(windowSeconds) * 1000          // window in ms
	expireSec := int64(windowSeconds) + 1            // TTL slightly above window
	member := fmt.Sprintf("%d:%d", now, time.Now().UnixNano()) // unique member per request

	key := "rate:" + identifier

	count, err := slidingWindowScript.Run(ctx, rdb, []string{key},
		now,       // ARGV[1]
		windowMs,  // ARGV[2]
		expireSec, // ARGV[3]
		member,    // ARGV[4]
	).Int64()

	if err != nil {
		return nil, fmt.Errorf("sliding window script error: %w", err)
	}

	return &SlidingWindowResult{
		Allowed:   count <= int64(limit),
		Count:     count,
		Limit:     limit,
		WindowSec: windowSeconds,
	}, nil
}
