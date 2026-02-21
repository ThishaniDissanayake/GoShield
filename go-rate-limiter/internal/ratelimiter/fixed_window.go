package ratelimiter

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// ────────────────────────────────────────────────────────────────────────
// Fixed-Window Rate Limiter — O(1) Time Complexity, Zero Race Conditions
// ────────────────────────────────────────────────────────────────────────
//
// Algorithm:
//   1. INCR  the key  →  O(1) atomic counter increment
//   2. If counter == 1 (first request in window), set EXPIRE  →  O(1)
//   3. Compare counter with limit  →  O(1)
//
// All three steps are packed into a single Lua script that Redis executes
// atomically — no other command can interleave between them.
//
// ┌────────────────────────────────────────────────────────────────────┐
// │ WHY O(1)?                                                         │
// │                                                                    │
// │  • Redis INCR operates on an integer in constant time.             │
// │  • Redis EXPIRE sets a TTL in constant time.                       │
// │  • The conditional check is a simple integer comparison.           │
// │  • No loops, no scans, no historical data — regardless of         │
// │    whether 1 or 1,000,000 requests have been made.                │
// └────────────────────────────────────────────────────────────────────┘
//
// ┌────────────────────────────────────────────────────────────────────┐
// │ WHY ZERO RACE CONDITIONS?                                         │
// │                                                                    │
// │  • The entire increment-check-expire sequence runs inside a       │
// │    single Lua script. Redis is single-threaded and executes Lua   │
// │    scripts atomically — no other client command can run in the    │
// │    middle.                                                         │
// │  • Even if 1000 requests arrive at the exact same millisecond,    │
// │    Redis queues them internally and processes each script         │
// │    invocation one-by-one. Every caller sees a unique, correct     │
// │    counter value.                                                  │
// │  • Unlike a naive  GET → local add → SET  pattern, INCR never    │
// │    produces duplicate or lost counts.                              │
// └────────────────────────────────────────────────────────────────────┘
//
// Architectural Significance:
//   • Constant time  → predictable latency & high throughput.
//   • Atomic script  → safe under concurrency (any number of GoShield
//     instances sharing one Redis).
//   • Stateless app  → horizontally scalable; all state lives in Redis.
// ────────────────────────────────────────────────────────────────────────

// fixedWindowScript performs INCR + conditional EXPIRE in a single atomic
// Lua execution. Returns the updated counter value.
//
// Time complexity per call: O(1)
// Race conditions:          None (atomic Lua script)
var fixedWindowScript = redis.NewScript(`
local key        = KEYS[1]
local expire_sec = tonumber(ARGV[1])

-- Step 1: Atomically increment the counter — O(1)
local count = redis.call("INCR", key)

-- Step 2: On the very first request in this window, set TTL — O(1)
if count == 1 then
    redis.call("EXPIRE", key, expire_sec)
end

-- Step 3: Return the counter so Go can compare with the limit — O(1)
return count
`)

// FixedWindowResult holds the outcome of a fixed-window rate-limit check.
type FixedWindowResult struct {
	Allowed   bool  // whether the request should be forwarded
	Count     int64 // current request count inside the window
	Limit     int   // configured maximum requests per window
	WindowSec int   // window duration in seconds
}

// CheckFixedWindow performs an O(1), race-condition-free rate-limit check
// for the given identifier using the fixed-window counter algorithm.
//
// Guarantees:
//   - O(1) time complexity: uses only Redis INCR and EXPIRE.
//   - Zero race conditions: all operations run in a single atomic Lua script.
//   - Safe at any scale: 1 or 100,000 concurrent callers see consistent results.
func CheckFixedWindow(ctx context.Context, rdb *redis.Client, identifier string, limit int, windowSeconds int) (*FixedWindowResult, error) {
	key := "rate:fixed:" + identifier

	count, err := fixedWindowScript.Run(ctx, rdb, []string{key},
		windowSeconds, // ARGV[1]
	).Int64()

	if err != nil {
		return nil, fmt.Errorf("fixed window script error: %w", err)
	}

	return &FixedWindowResult{
		Allowed:   count <= int64(limit),
		Count:     count,
		Limit:     limit,
		WindowSec: windowSeconds,
	}, nil
}
