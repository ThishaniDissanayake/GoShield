# GoShield

GoShield is a lightweight Golang + Redis microservice that enforces per-client API rate limits. It can run as a standalone gateway or be embedded as middleware inside an existing Gin application, providing fast protection against abusive traffic bursts while keeping implementation simple.

## Why GoShield?

- **High throughput:** Uses Redis atomic counters (`INCR` + `EXPIRE`) for O(1) request tracking.
- **Configurable throttling:** Limit and window can be tuned through environment variables for each deployment.
- **Drop-in middleware:** One line to clamp routes; return codes follow HTTP 429 semantics with JSON payloads.
- **Cloud-friendly:** Ships with Docker/Docker Compose setups and is stateless apart from Redis.

## Architecture Snapshot

| Component | Responsibility |
| --- | --- |
| `cmd/server/main.go` | Boots Gin, loads env vars, wires middleware & health route. |
| `internal/config/redis.go` | Creates and validates the Redis client. |
| `internal/middleware/ratelimiter.go` | Implements the sliding-window counter and emits 429 responses. |
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
```

All keys have sane defaults; only override what you need. `RATE_LIMIT` is the number of requests allowed per IP during the `WINDOW_SECONDS` interval.

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
