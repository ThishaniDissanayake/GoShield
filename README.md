# GoShield

GoShield is a lightweight Golang + Redis microservice that enforces per-client API rate limits. It can run as a standalone gateway or be embedded as middleware inside an existing Gin application, providing fast protection against abusive traffic bursts while keeping implementation simple.

## Why GoShield?

- **O(1) time complexity:** Every request completes in constant time — no loops, no scans, no degradation under load.
- **Zero race conditions:** All counter operations run inside atomic Redis Lua scripts — safe at any concurrency level.
- **Dual algorithm:** Choose between fixed-window (`INCR + EXPIRE`) or sliding-window (`ZSET`) via a single env var.
- **Configurable throttling:** Limit and window can be tuned through environment variables for each deployment.
- **Drop-in middleware:** One line to clamp routes; return codes follow HTTP 429 semantics with JSON payloads.
- **Cloud-friendly:** Ships with Docker/Docker Compose setups and is stateless apart from Redis.

---

## O(1) Time Complexity & Zero Race Conditions

This section explains the core performance and safety guarantees of GoShield.

### O(1) Time Complexity

Each incoming request performs a **constant number of Redis operations** regardless of:

- How many users are in the system
- How many requests have been made previously
- How much traffic is hitting the API

#### Fixed-Window Mode (`RATE_LIMIT_MODE=fixed`)

```
Request arrives
    │
    ▼
┌─────────────────────────────────────────┐
│  Lua Script (atomic, single Redis call) │
│                                         │
│  1. INCR key          →  O(1)           │
│  2. EXPIRE key (if 1) →  O(1)           │
│  3. Return count      →  O(1)           │
└─────────────────────────────────────────┘
    │
    ▼
  Compare count vs limit  →  O(1)
    │
    ▼
  Allow (200) or Block (429)
```

**Total: O(1)** — three constant-time Redis commands + one comparison.

#### Sliding-Window Mode (`RATE_LIMIT_MODE=sliding`)

```
Request arrives
    │
    ▼
┌──────────────────────────────────────────────────┐
│  Lua Script (atomic, single Redis call)          │
│                                                  │
│  1. ZREMRANGEBYSCORE  →  O(log N + M) *          │
│  2. ZADD              →  O(log N)     *          │
│  3. ZCARD             →  O(1)                    │
│  4. EXPIRE            →  O(1)                    │
│                                                  │
│  * N ≤ RATE_LIMIT (bounded), so effectively O(1) │
└──────────────────────────────────────────────────┘
    │
    ▼
  Compare count vs limit  →  O(1)
    │
    ▼
  Allow (200) or Block (429)
```

**Total: Amortised O(1)** — the sorted set never grows beyond `RATE_LIMIT` entries, keeping `log N` trivially small.

#### Why This Matters

| Scenario | Operations per request | Latency change |
|---|---|---|
| 1 user, 1 request | INCR + EXPIRE + compare | Baseline |
| 1,000 users, 1,000 requests | INCR + EXPIRE + compare (each) | **Same** |
| 100,000 users, 100,000 requests | INCR + EXPIRE + compare (each) | **Same** |

No loops. No scanning of historical data. No complex computations. Predictable performance at any scale.

### Zero Race Conditions

#### The Problem

A naive rate limiter might do:

```
1. GET counter         →  read current value (e.g. 99)
2. counter += 1        →  add locally (100)
3. SET counter 100     →  write back
```

If two requests execute this simultaneously:

```
Request A: GET → 99    Request B: GET → 99
Request A: SET → 100   Request B: SET → 100  ← WRONG! Should be 101
```

The counter is corrupted. One request was effectively invisible.

#### GoShield's Solution

GoShield uses **Redis atomic operations** via Lua scripts:

```
┌──────────────────────────────────────────────────────────┐
│  Redis Lua Script Execution (Single-Threaded)            │
│                                                          │
│  Request A arrives → Lua script runs to completion       │
│                      counter: 99 → 100                   │
│                      returns 100                         │
│                                                          │
│  Request B arrives → Lua script runs to completion       │
│                      counter: 100 → 101                  │
│                      returns 101  ✓ CORRECT              │
│                                                          │
│  No interleaving. No partial reads. No lost updates.     │
└──────────────────────────────────────────────────────────┘
```

**Why this works:**

1. **Redis INCR** is an atomic increment — it reads, increments, and writes in a single indivisible operation.
2. **Lua scripts** execute atomically — Redis will not run any other command until the script completes.
3. **Single-threaded execution** — Redis processes commands sequentially; no two scripts ever run concurrently.

Even if 1,000 requests arrive at the same nanosecond, Redis queues them and processes each one sequentially. Every caller sees a unique, correct counter value.

### Architectural Significance

```
                    ┌─────────────────────────┐
                    │   Load Balancer          │
                    └────┬──────────┬──────────┘
                         │          │
              ┌──────────▼──┐  ┌───▼───────────┐
              │ GoShield #1 │  │ GoShield #2    │
              │ (stateless) │  │ (stateless)    │
              └──────┬──────┘  └───┬────────────┘
                     │             │
                     ▼             ▼
              ┌────────────────────────┐
              │   Redis (shared)       │
              │   Atomic Lua scripts   │
              │   O(1) per operation   │
              └────────────────────────┘
```

| Property | Guarantee | Mechanism |
|---|---|---|
| **O(1) per request** | Execution time never grows with traffic | Redis INCR / bounded ZSET |
| **Zero race conditions** | Counter is always accurate | Atomic Lua scripts |
| **Horizontal scalability** | Add GoShield instances freely | Stateless app, shared Redis |
| **Memory safety** | Keys auto-expire | TTL on every key |
| **Production readiness** | Safe under any concurrency | Single-threaded Redis execution |

## Architecture Snapshot

| Component | Responsibility |
| --- | --- |
| `cmd/server/main.go` | Boots Gin, loads env vars, wires middleware & health route (middleware mode). |
| `cmd/gateway/main.go` | Boots Gin, creates reverse proxy, applies rate limiting (gateway mode). |
| `internal/config/redis.go` | Creates and validates the Redis client. |
| `internal/ratelimiter/fixed_window.go` | O(1) fixed-window algorithm — atomic Lua script (INCR + EXPIRE). |
| `internal/ratelimiter/sliding_window.go` | Sliding-window algorithm — atomic Lua script (ZSET operations). |
| `internal/middleware/ratelimiter.go` | Gin middleware that delegates to fixed or sliding limiter. |
| `internal/gateway/proxy.go` | Reverse proxy helper for gateway mode. |
| `internal/handlers/health.go` | Simple readiness probe returning `{"status":"OK"}`. |

Request flow: client → Gin router → rate limiter middleware (Redis check) → downstream handler (or 429). All state (counters) lives in Redis, so multiple instances can run behind a load balancer without coordination.

## Getting Started

### Prerequisites

- Go 1.21+ installed (`go version`)
- Redis 6+ reachable at `localhost:6379` (run via Docker: `docker run -d -p 6379:6379 redis`)

### Environment Variables

Create a `.env` file in the repo root:

```
RATE_LIMIT=100
WINDOW_SECONDS=60
RATE_LIMIT_MODE=sliding
```

| Variable | Default | Description |
|---|---|---|
| `RATE_LIMIT` | `100` | Max requests per IP per window |
| `WINDOW_SECONDS` | `60` | Window duration in seconds |
| `RATE_LIMIT_MODE` | `sliding` | Algorithm: `sliding` (ZSET) or `fixed` (INCR) |
| `REDIS_ADDR` | `redis:6379` | Redis connection address |
| `UPSTREAM_URL` | — | Upstream URL (required in gateway mode) |

All keys have sane defaults; only override what you need.

### Run Locally

```bash
# install dependencies
go mod tidy

# start the service
go run cmd/server/main.go

# ping the health endpoint
curl http://localhost:8080/health
```

Trigger a rate-limit response by firing more than `RATE_LIMIT` requests within the configured window to any protected route; you will receive HTTP 429 with `{ "error": "Too many requests" }`.

## Docker & Compose

```
docker compose up --build
```

This spins up both Redis and the GoShield container using the environment defined in `.env` (compose file expected at `go-rate-limiter/docker-compose.yml`).

## Extending GoShield

- **Different identifiers:** Swap `c.ClientIP()` with API keys or JWT subject IDs.
- **Route-specific limits:** Pass different limit/window pairs when attaching middleware to selected routes.
- **Observability:** Add metrics/logging hooks in the middleware to ship data to Prometheus, OpenTelemetry, etc.

## Testing Checklist

- Rates reset after `WINDOW_SECONDS` as verified via Redis TTL.
- Health endpoint returns 200 while Redis is reachable, otherwise service exits on startup.
- Dockerized deployment can be scaled horizontally; counters remain accurate due to Redis centralization.

## Roadmap Ideas

- Token bucket / leaky bucket strategies for smoother bursts.
- Distributed tracing + structured logging.
- Admin endpoint for clearing keys and viewing per-IP usage.

---

MIT Licensed © 2026 Thishani Dissanayake
