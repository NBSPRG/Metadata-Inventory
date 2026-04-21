# HTTP Metadata Inventory Service — Production-Grade Implementation Plan

> **Status**: Planning Phase  
> **Language**: Go 1.22+  
> **Challenge Level**: Senior Backend SDE (Production Ready)  
> **Last Updated**: 2026-04-21

---

## Table of Contents

1. [Project Philosophy](#1-project-philosophy)
2. [Architecture Overview](#2-architecture-overview)
3. [Technology Stack & Justification](#3-technology-stack--justification)
4. [Project Structure](#4-project-structure)
5. [System Design Decisions](#5-system-design-decisions)
6. [Feature Flags & Backward Compatibility](#6-feature-flags--backward-compatibility)
7. [API Design](#7-api-design)
8. [Data Layer Design](#8-data-layer-design)
9. [Kafka & Async Worker Design](#9-kafka--async-worker-design)
10. [Observability: Logging, Metrics, Tracing](#10-observability-logging-metrics-tracing)
11. [Error Handling Strategy](#11-error-handling-strategy)
12. [Security](#12-security)
13. [Testing Strategy](#13-testing-strategy)
14. [Docker & Infrastructure](#14-docker--infrastructure)
15. [Documentation Standards](#15-documentation-standards)
16. [Implementation Phases](#16-implementation-phases)
17. [Upgrade & Degradation Paths](#17-upgrade--degradation-paths)
18. [Interviewer Impression Points](#18-interviewer-impression-points)

---

## 1. Project Philosophy

This service is built under the following non-negotiable principles:

| Principle | What It Means Here |
|---|---|
| **Explicit over magic** | No hidden framework behavior. Every dependency is injected, every error is handled. |
| **Interface-first design** | Every layer talks to an interface, never a concrete type. Swap implementations freely. |
| **Context propagation** | Every I/O call carries a `context.Context`. Cancellation and deadlines flow end-to-end. |
| **Observability is not optional** | Structured logs, metrics, and trace IDs are built in from day one — not added later. |
| **Fail fast, recover gracefully** | Services validate config at startup. Panics are caught at boundaries. |
| **Backward compatibility** | New behavior is gated behind feature flags. Old clients are never broken. |
| **Twelve-Factor App** | Config from env, stateless processes, logs to stdout, explicit deps. |

---

## 2. Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────┐
│                          Client / Consumer                          │
└───────────────────────────────────┬─────────────────────────────────┘
                                    │ HTTP/REST
                                    ▼
┌─────────────────────────────────────────────────────────────────────┐
│                        API Service (Go)                             │
│                                                                     │
│  ┌──────────┐   ┌──────────┐   ┌──────────────┐   ┌─────────────┐ │
│  │ Router   │──▶│ Handler  │──▶│ Service Layer│──▶│ Repository  │ │
│  │ (chi)    │   │ (HTTP)   │   │ (Business)   │   │ (Interface) │ │
│  └──────────┘   └──────────┘   └──────┬───────┘   └──────┬──────┘ │
│                                        │                   │        │
│                              Cache Miss│                   │        │
│                                        ▼                   ▼        │
│                              ┌──────────────┐     ┌──────────────┐ │
│                              │Kafka Producer│     │  MongoDB     │ │
│                              └──────┬───────┘     └──────────────┘ │
└─────────────────────────────────────┼───────────────────────────────┘
                                      │
                         Topic: url.fetch.requested
                                      │
┌─────────────────────────────────────▼───────────────────────────────┐
│                       Worker Service (Go)                           │
│                                                                     │
│  ┌──────────────┐   ┌──────────────┐   ┌──────────────────────────┐│
│  │Kafka Consumer│──▶│Fetch Pipeline│──▶│  Repository (Interface)  ││
│  └──────────────┘   └──────────────┘   └──────────────────────────┘│
└─────────────────────────────────────────────────────────────────────┘
                                      │
                    ┌─────────────────┼─────────────────┐
                    ▼                 ▼                   ▼
             ┌────────────┐  ┌──────────────┐  ┌────────────────┐
             │  MongoDB   │  │  Prometheus  │  │  OpenTelemetry │
             │            │  │  /metrics    │  │  (Traces)      │
             └────────────┘  └──────────────┘  └────────────────┘
```

### Service Responsibilities

| Service | Responsibility |
|---|---|
| **API Service** | Accept HTTP requests, serve cached data, dispatch async jobs |
| **Worker Service** | Consume Kafka events, fetch URLs, persist metadata |
| **Shared `pkg/`** | DB, Kafka, fetcher, config, logging — reusable, tested independently |

---

## 3. Technology Stack & Justification

### Core

| Technology | Version | Justification |
|---|---|---|
| **Go** | 1.22+ | Explicit concurrency, minimal runtime, ideal for I/O-heavy services |
| **chi** | v5 | Lightweight, idiomatic HTTP router; composable middleware; no magic |
| **MongoDB** | 7.x | Flexible schema for heterogeneous HTTP metadata; native document model |
| **Kafka** | 3.x (KRaft mode) | Durable async dispatch; decouples API from fetch latency |
| **Docker Compose** | v2 | Local dev parity with production topology |

### Observability

| Tool | Purpose |
|---|---|
| **`log/slog`** (stdlib) | Structured JSON logging — no external dep needed |
| **Prometheus** | Metrics scraping via `/metrics` endpoint |
| **OpenTelemetry SDK** | Distributed tracing (trace IDs propagated via HTTP headers) |
| **Grafana** (optional in compose) | Metrics dashboard for local dev |

### Developer Experience

| Tool | Purpose |
|---|---|
| **`godotenv`** | `.env` file loading for local dev |
| **`go-playground/validator`** | Struct-level input validation with tags |
| **`swaggo/swag`** | Auto-generated OpenAPI 3.0 / Swagger UI |
| **`testify`** | Assertions + mocking in tests |
| **`testcontainers-go`** | Real MongoDB + Kafka in integration tests |
| **`golangci-lint`** | Linting pipeline |

### Why Not Gin/Echo?
`chi` is chosen because it is 100% `net/http` compatible — handlers are standard `http.Handler`. No lock-in. Middleware is composable. Migrating to any other router (or none) is trivial.

---

## 4. Project Structure

```
metadata-inventory/
│
├── api/                          # API service binary
│   ├── main.go                   # Entry point: wire deps, start server
│   ├── server/
│   │   ├── server.go             # HTTP server setup, graceful shutdown
│   │   └── routes.go             # Route registration
│   ├── handlers/
│   │   ├── metadata_post.go      # POST /metadata
│   │   ├── metadata_get.go       # GET /metadata
│   │   ├── health.go             # GET /health, GET /ready
│   │   └── handler_test.go       # Handler-level tests
│   └── Dockerfile
│
├── worker/                       # Worker service binary
│   ├── main.go                   # Entry point: wire deps, start consumer
│   ├── consumer/
│   │   ├── consumer.go           # Kafka consumer loop
│   │   └── consumer_test.go
│   └── Dockerfile
│
├── pkg/                          # Shared internal packages (not exported)
│   │
│   ├── config/
│   │   ├── config.go             # Config struct, Load() from env
│   │   └── config_test.go
│   │
│   ├── db/
│   │   ├── client.go             # MongoDB connection with retry
│   │   ├── models.go             # MetadataRecord struct (BSON + JSON tags)
│   │   ├── repository.go         # MetadataRepository interface
│   │   ├── mongo_repository.go   # MongoDB implementation
│   │   ├── mock_repository.go    # Mock for unit tests
│   │   └── repository_test.go
│   │
│   ├── kafka/
│   │   ├── producer.go           # EventProducer interface + implementation
│   │   ├── consumer.go           # EventConsumer base
│   │   ├── messages.go           # Typed message structs
│   │   └── kafka_test.go
│   │
│   ├── fetcher/
│   │   ├── fetcher.go            # Fetcher interface
│   │   ├── http_fetcher.go       # HTTP implementation
│   │   ├── mock_fetcher.go       # Mock for unit tests
│   │   └── fetcher_test.go
│   │
│   ├── service/
│   │   ├── metadata_service.go   # Core business logic (interface)
│   │   ├── metadata_service_impl.go
│   │   └── service_test.go
│   │
│   ├── middleware/
│   │   ├── logging.go            # Request/response structured logging
│   │   ├── tracing.go            # OpenTelemetry trace injection
│   │   ├── recovery.go           # Panic recovery → 500
│   │   ├── request_id.go         # X-Request-ID propagation
│   │   └── ratelimit.go          # Optional rate limiter (feature-flagged)
│   │
│   ├── observability/
│   │   ├── logger.go             # Global slog setup
│   │   ├── metrics.go            # Prometheus metric definitions
│   │   └── tracer.go             # OTel tracer setup
│   │
│   └── featureflags/
│       ├── flags.go              # Feature flag interface + env-based impl
│       └── flags_test.go
│
├── docs/
│   ├── swagger/                  # Auto-generated by swaggo
│   ├── adr/                      # Architecture Decision Records
│   │   ├── ADR-001-language-choice.md
│   │   ├── ADR-002-kafka-for-async.md
│   │   ├── ADR-003-mongodb-schema.md
│   │   └── ADR-004-feature-flags.md
│   └── runbook.md                # Ops runbook
│
├── scripts/
│   ├── wait-for-it.sh            # Service readiness probe
│   └── seed.sh                   # Optional: seed test data
│
├── tests/
│   └── integration/
│       ├── api_test.go           # Full flow integration tests
│       └── worker_test.go
│
├── docker-compose.yml            # Full stack: API + Worker + Mongo + Kafka
├── docker-compose.test.yml       # Test stack (testcontainers alternative)
├── .env.example                  # All required env vars documented
├── .env                          # Local dev values (gitignored)
├── Makefile                      # Dev commands
├── go.work                       # Go workspace for monorepo
├── go.mod
├── go.sum
├── .golangci.yml                 # Linter config
└── README.md
```

---

## 5. System Design Decisions

### 5.1 Interface-Driven Architecture

Every cross-layer dependency is defined as a Go interface. Concrete implementations are injected at startup (`main.go` is the only place that imports concrete types).

```
Handler → ServiceInterface → RepositoryInterface
                          → ProducerInterface
                          → FetcherInterface
```

**Benefit**: Swap MongoDB for Postgres, Kafka for RabbitMQ, or mock any layer — zero changes to business logic.

### 5.2 Dependency Injection (Manual, No Framework)

No DI frameworks (Wire is optional). All wiring happens in `main.go` or a `wire.go` file per binary. This is explicit, readable, and easy to follow during code review.

### 5.3 Repository Pattern

The `MetadataRepository` interface defines the data contract:

```go
type MetadataRepository interface {
    FindByURL(ctx context.Context, url string) (*MetadataRecord, error)
    Upsert(ctx context.Context, record *MetadataRecord) error
    UpdateStatus(ctx context.Context, url string, status Status, result *FetchResult) error
}
```

Business logic never imports `mongo-driver` directly.

### 5.4 Service Layer

All business logic lives in `pkg/service/`. Handlers are thin — they parse input, call service, write response. Services are thin — they orchestrate, not implement.

```
Handler: parse request → call service → write response
Service: check cache → dispatch or return → no HTTP, no DB drivers
Repository: DB operations only
```

### 5.5 Graceful Shutdown

Both services listen for `SIGTERM`/`SIGINT`. On signal:
1. Stop accepting new requests / Kafka messages
2. Wait for in-flight work to complete (configurable drain timeout)
3. Flush logs and close connections

### 5.6 Circuit Breaker (Planned / Feature-Flagged)

Wrap the HTTP fetcher with a circuit breaker (`sony/gobreaker`) gated behind a feature flag. If a domain is consistently failing, open the circuit to avoid hammering it.

### 5.7 Idempotency

- POST is idempotent: if URL already exists with `status: ready`, return existing record with `200 OK` (not re-fetch).
- Worker upserts by URL — running twice is safe.
- Kafka consumer uses manual offset commit after successful DB write.

---

## 6. Feature Flags & Backward Compatibility

### Feature Flag Interface

```go
type FeatureFlags interface {
    IsEnabled(flag Flag) bool
}
```

Initial implementation: env-var based (`FF_RATE_LIMIT_ENABLED=true`). Can be swapped for LaunchDarkly, Unleash, or a DB-backed config table — same interface.

### Defined Flags

| Flag | Default | Purpose |
|---|---|---|
| `FF_RATE_LIMIT_ENABLED` | `false` | Enable per-IP rate limiting on API |
| `FF_CIRCUIT_BREAKER_ENABLED` | `false` | Enable circuit breaker on HTTP fetcher |
| `FF_PAGE_SOURCE_STORAGE` | `true` | Store page source (disable to reduce DB size) |
| `FF_ASYNC_FETCH_ONLY` | `false` | Force all POST requests to also go async |
| `FF_METRICS_ENABLED` | `true` | Enable Prometheus metrics endpoint |
| `FF_TRACING_ENABLED` | `false` | Enable OpenTelemetry tracing |

### API Versioning Strategy

All endpoints are prefixed `/v1/`. When breaking changes are needed:
- New behavior ships under `/v2/`
- `/v1/` continues to work unchanged
- Deprecation header added: `Deprecation: true`, `Sunset: <date>`
- Both versions can run simultaneously — the router handles it

### Backward-Compatible DB Changes

- New fields are always optional with defaults
- Never delete/rename a field — add new, migrate data, deprecate old
- MongoDB schema versions tracked in `schema_version` field per document

---

## 7. API Design

### Endpoint Specifications

#### `POST /v1/metadata`

```
Request:
  Content-Type: application/json
  Body: { "url": "https://example.com" }

Response 201:
  { "id": "...", "url": "...", "status": "ready", "headers": {...},
    "cookies": [...], "fetched_at": "...", "created_at": "..." }

Response 200 (already exists):
  Same body — idempotent

Response 400:
  { "error": "INVALID_URL", "message": "URL must be absolute with http(s) scheme" }

Response 422:
  { "error": "FETCH_FAILED", "message": "Could not reach target URL", "details": "..." }

Response 500:
  { "error": "INTERNAL_ERROR", "message": "An unexpected error occurred",
    "request_id": "..." }
```

#### `GET /v1/metadata?url=https://example.com`

```
Response 200 (cache hit):
  Same as POST 201 body

Response 202 (cache miss — async dispatched):
  { "status": "pending", "url": "...", "message": "Fetch request queued",
    "poll_after_seconds": 5 }

Response 400:
  { "error": "MISSING_PARAM", "message": "url query parameter is required" }
```

#### `GET /health` (liveness)
```
Response 200: { "status": "ok", "timestamp": "..." }
```

#### `GET /ready` (readiness — checks DB + Kafka)
```
Response 200: { "status": "ready", "checks": { "mongodb": "ok", "kafka": "ok" } }
Response 503: { "status": "degraded", "checks": { "mongodb": "ok", "kafka": "error" } }
```

### Consistent Error Response Format

All errors follow this schema — every endpoint, every error code:

```json
{
  "error": "ERROR_CODE",
  "message": "Human-readable description",
  "request_id": "uuid",
  "details": "optional extra context"
}
```

### Request ID Propagation

- Every request gets a `X-Request-ID` header (generated if not provided)
- This ID is attached to all log lines for that request
- Included in all error responses
- Propagated to Kafka messages and worker logs — full trace across services

---

## 8. Data Layer Design

### MongoDB Collection: `metadata`

```json
{
  "_id":            "ObjectId",
  "url":            "string — unique index, hashed for fast lookup",
  "url_hash":       "string — SHA-256 of url, for exact-match index",
  "status":         "enum: pending | fetching | ready | failed",
  "schema_version": "int — for future migrations",
  "headers":        "object — map<string, []string>",
  "cookies": [
    { "name": "string", "value": "string", "domain": "string",
      "path": "string", "http_only": "bool", "secure": "bool" }
  ],
  "page_source":    "string — raw HTML (feature-flagged)",
  "page_source_size_bytes": "int",
  "error":          "string — set if status=failed",
  "retry_count":    "int",
  "fetch_duration_ms": "int",
  "fetched_at":     "ISODate",
  "created_at":     "ISODate",
  "updated_at":     "ISODate"
}
```

### Indexes

```
{ url: 1 }           — unique, sparse
{ url_hash: 1 }      — unique, for O(1) lookup
{ status: 1 }        — for worker queue visibility
{ created_at: -1 }   — for time-based listing (future)
{ updated_at: -1 }   — TTL index candidate (future)
```

### Connection Resilience

MongoDB client configured with:
- `ServerSelectionTimeout`: 5s
- `ConnectTimeout`: 10s
- `MaxPoolSize`: 50
- Retry on startup with exponential backoff (max 5 attempts)

---

## 9. Kafka & Async Worker Design

### Topic Configuration

```
Topic:              url.fetch.requested
Partitions:         6  (allows 6 parallel worker instances)
Replication Factor: 1  (local dev) / 3 (production)
Retention:          24h
```

### Message Schema (versioned)

```json
{
  "version":      "1",
  "url":          "https://example.com",
  "requested_at": "2026-04-21T10:00:00Z",
  "request_id":   "uuid — for tracing",
  "source":       "api-get | api-post"
}
```

Message version field enables schema evolution. Worker ignores unknown fields (forward-compatible).

### Consumer Design

- Consumer group: `metadata-worker-v1`
- **Manual offset commit**: offset committed only after successful DB write
- **At-least-once delivery**: idempotent upserts make duplicate processing safe
- **Dead Letter Topic**: `url.fetch.failed` — messages that fail after N retries
- **Backpressure**: worker processes one message at a time per partition (configurable concurrency)
- **Poison pill handling**: malformed messages logged and skipped, not retried forever

### Worker Pipeline

```
Kafka Message Received
        │
        ▼
Validate & Deserialize
        │
        ▼
Mark DB status: "fetching"  ← prevents duplicate concurrent fetches
        │
        ▼
HTTP Fetch (with timeout + circuit breaker)
        │
   ┌────┴────┐
Success    Failure
   │           │
   ▼           ▼
Update DB   Update DB
status:     status:
"ready"     "failed"
   │           │
Commit     Log + DLT
Offset     + Commit
```

---

## 10. Observability: Logging, Metrics, Tracing

### Structured Logging (`log/slog`)

All logs are JSON, written to stdout. Never use `fmt.Println` in production code.

**Standard fields on every log line:**

```json
{
  "time":       "ISO8601",
  "level":      "INFO|WARN|ERROR",
  "service":    "api|worker",
  "version":    "1.0.0",
  "request_id": "uuid",
  "trace_id":   "otel-trace-id",
  "msg":        "human message",
  "url":        "context-specific fields..."
}
```

**Log levels:**
- `DEBUG`: Request details, DB queries (disabled in production)
- `INFO`: Request received/completed, background job dispatched/completed
- `WARN`: Retries, degraded dependencies, feature flag overrides
- `ERROR`: Unrecoverable failures, panics caught, external service errors

**Never log**: passwords, auth tokens, full cookie values, PII.

### Prometheus Metrics

Exposed at `GET /metrics` (feature-flagged).

| Metric | Type | Labels |
|---|---|---|
| `http_requests_total` | Counter | method, path, status_code |
| `http_request_duration_seconds` | Histogram | method, path |
| `metadata_cache_hits_total` | Counter | — |
| `metadata_cache_misses_total` | Counter | — |
| `fetch_duration_seconds` | Histogram | status (success/failure) |
| `kafka_messages_produced_total` | Counter | topic |
| `kafka_messages_consumed_total` | Counter | topic, status |
| `worker_fetch_errors_total` | Counter | error_type |
| `mongodb_operation_duration_seconds` | Histogram | operation |

### OpenTelemetry Tracing

- Trace spans created for: HTTP request, DB operation, Kafka produce, HTTP fetch
- `X-Trace-ID` injected into responses for client-side correlation
- Feature-flagged: `FF_TRACING_ENABLED` — zero cost when disabled

---

## 11. Error Handling Strategy

### Sentinel Errors

Define typed error constants in `pkg/apperrors/`:

```go
var (
    ErrNotFound       = errors.New("not found")
    ErrInvalidURL     = errors.New("invalid url")
    ErrFetchFailed    = errors.New("fetch failed")
    ErrAlreadyPending = errors.New("already pending")
    ErrDatabaseError  = errors.New("database error")
)
```

Handlers use `errors.Is()` to map domain errors to HTTP status codes. Business logic never returns HTTP status codes.

### Error Wrapping

Use `fmt.Errorf("operation context: %w", err)` throughout. Callers can unwrap to inspect cause without losing context.

### Panic Recovery

Middleware catches any panic, logs the stack trace with request ID, and returns a structured `500` response. Service never crashes due to a bad request.

---

## 12. Security

| Concern | Implementation |
|---|---|
| **Input validation** | URL must be absolute, scheme must be `http` or `https`, max length 2048 chars |
| **SSRF prevention** | Block private IP ranges (10.x, 172.16.x, 192.168.x, 127.x, ::1) |
| **Secrets management** | All credentials via env vars — never in code or docker-compose defaults |
| **Header sanitization** | Strip `Set-Cookie` and `Authorization` values from stored headers |
| **Rate limiting** | Per-IP rate limiting, feature-flagged |
| **Container security** | Non-root user in Dockerfiles, read-only filesystem where possible |
| **Dependency audit** | `govulncheck` in CI pipeline |

---

## 13. Testing Strategy

### Test Pyramid

```
       ┌──────────────┐
       │   E2E Tests  │  ← docker-compose up, real HTTP
       ├──────────────┤
       │ Integration  │  ← testcontainers (real Mongo + Kafka)
       ├──────────────┤
       │  Unit Tests  │  ← mocks, pure logic (80% coverage target)
       └──────────────┘
```

### Unit Tests

- Every package has `_test.go` files
- Repository tested against `mock_repository.go`
- Fetcher tested against `httptest.Server`
- Service tested against mocked repository + producer + fetcher
- Table-driven tests for all validation logic

### Integration Tests (`tests/integration/`)

Using `testcontainers-go`:
```
- Start real MongoDB container
- Start real Kafka container
- Run POST → verify DB document
- Run GET (cache hit) → verify 200
- Run GET (cache miss) → verify 202 + Kafka message published
- Start worker → consume message → verify DB updated
- Run GET again → verify 200 with data
```

### Test Utilities

- `testhelpers/` package: factory functions for test data, DB seeding helpers
- `assert` package from `testify` for readable assertions
- `-race` flag always enabled: `go test -race ./...`

### CI Pipeline (GitHub Actions)

```yaml
jobs:
  lint:     golangci-lint
  test:     go test -race -cover ./...
  security: govulncheck ./...
  build:    docker build api/ && docker build worker/
  e2e:      docker-compose -f docker-compose.test.yml up --abort-on-container-exit
```

---

## 14. Docker & Infrastructure

### docker-compose.yml Services

```yaml
services:
  zookeeper:
    image: confluentinc/cp-zookeeper:7.5.0
    healthcheck: zkOk

  kafka:
    image: confluentinc/cp-kafka:7.5.0
    depends_on: [zookeeper]
    healthcheck: kafka-topics list
    environment:
      KAFKA_AUTO_CREATE_TOPICS_ENABLE: "true"

  mongo:
    image: mongo:7.0
    healthcheck: mongosh --eval "db.adminCommand('ping')"
    volumes:
      - mongo_data:/data/db

  api:
    build: ./api
    depends_on:
      mongo: { condition: service_healthy }
      kafka: { condition: service_healthy }
    ports: ["8080:8080"]
    env_file: .env
    healthcheck: curl /health

  worker:
    build: ./worker
    depends_on:
      mongo: { condition: service_healthy }
      kafka: { condition: service_healthy }
    env_file: .env
    deploy:
      replicas: 2        # Horizontal scaling out of the box

  prometheus:             # Optional — metrics dashboard
    image: prom/prometheus
    volumes:
      - ./config/prometheus.yml:/etc/prometheus/prometheus.yml

volumes:
  mongo_data:
```

### Dockerfile (Multi-Stage)

```dockerfile
# Build stage
FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download                    # Cache deps layer
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /bin/api ./api

# Runtime stage — minimal image
FROM scratch
COPY --from=builder /bin/api /api
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
USER 65534:65534                        # Non-root
EXPOSE 8080
ENTRYPOINT ["/api"]
```

**Result**: ~10MB final image. No shell, no OS utilities — minimal attack surface.

---

## 15. Documentation Standards

### README.md Structure

```
1. What this service does (2 sentences)
2. Quick Start (docker-compose up — working in 60 seconds)
3. API Reference (with curl examples)
4. Configuration (all env vars, defaults, descriptions)
5. Architecture (link to diagram)
6. Development Guide (make targets, test commands)
7. Feature Flags Reference
8. Runbook (common operational tasks)
```

### Architecture Decision Records (ADRs)

Every significant design choice gets an ADR in `docs/adr/`. Format:

```markdown
# ADR-001: Use Go over Kotlin/Spring Boot

## Status: Accepted

## Context
Challenge requires async worker design. Evaluating language fit.

## Decision
Go chosen for explicit concurrency model (goroutines), smaller binary,
and idiomatic fit for I/O-heavy service.

## Consequences
- No OOP inheritance — composition via interfaces
- Manual DI — more verbose main.go, more explicit
- Smaller Docker image, faster startup
```

### Code Documentation

- All exported types, functions, and interfaces have Go doc comments
- Complex logic has inline comments explaining **why**, not **what**
- `pkg/` packages each have a `doc.go` with package-level documentation

### Swagger / OpenAPI

Auto-generated from Go doc annotations via `swaggo/swag`. Served at `/swagger/` in development. Exported as `docs/swagger/openapi.json` for sharing.

---

## 16. Implementation Phases

Each phase is independently deployable and testable before moving to the next.

---

### Phase 0 — Foundation (Day 1)

**Goal**: `docker-compose up` starts all containers. Services boot, log, and exit cleanly.

- [ ] Initialize Go workspace (`go.work`, `go.mod` per service)
- [ ] `pkg/config/` — Config struct, `Load()` from env, validation at startup
- [ ] `pkg/observability/` — Logger setup (JSON slog), Prometheus registry, OTel tracer stub
- [ ] `docker-compose.yml` — All 5 services wired with health checks
- [ ] `Makefile` — `make up`, `make down`, `make test`, `make lint`, `make swagger`
- [ ] `.env.example` — Every variable documented
- [ ] Bare `main.go` for both `api/` and `worker/` — logs "starting..." and exits

**Test**: `docker-compose up` → all services healthy → `make down`

---

### Phase 1 — MongoDB Layer (Day 1-2)

**Goal**: Can store and retrieve metadata records. Fully unit tested.

- [ ] `pkg/db/models.go` — `MetadataRecord` struct with BSON + JSON tags + `Status` enum
- [ ] `pkg/db/repository.go` — `MetadataRepository` interface
- [ ] `pkg/db/mongo_repository.go` — Concrete implementation
- [ ] `pkg/db/mock_repository.go` — Mock for unit tests
- [ ] `pkg/db/client.go` — Connection with exponential backoff retry
- [ ] Index creation on startup (`url` unique, `url_hash`, `status`)
- [ ] Unit tests with mock + integration test with testcontainers

**Test**: `go test -race ./pkg/db/...`

---

### Phase 2 — Kafka Layer (Day 2)

**Goal**: Can produce and consume typed messages. Fully unit tested.

- [ ] `pkg/kafka/messages.go` — `FetchRequestMessage` struct (versioned)
- [ ] `pkg/kafka/producer.go` — `EventProducer` interface + implementation
- [ ] `pkg/kafka/consumer.go` — `EventConsumer` base (reusable)
- [ ] Connection retry logic
- [ ] Unit tests (mock producer) + integration test (testcontainers Kafka)

**Test**: `go test -race ./pkg/kafka/...`

---

### Phase 3 — HTTP Fetcher (Day 2-3)

**Goal**: Can fetch headers, cookies, and page source for any URL.

- [ ] `pkg/fetcher/fetcher.go` — `Fetcher` interface + `FetchResult` struct
- [ ] `pkg/fetcher/http_fetcher.go` — Implementation with timeout, redirect handling
- [ ] SSRF protection — block private IPs
- [ ] `pkg/fetcher/mock_fetcher.go` — Mock for service tests
- [ ] Feature-flagged page source storage
- [ ] Unit tests with `httptest.Server`

**Test**: `go test -race ./pkg/fetcher/...`

---

### Phase 4 — Service Layer (Day 3)

**Goal**: Business logic fully testable in isolation with no real dependencies.

- [ ] `pkg/service/metadata_service.go` — `MetadataService` interface
- [ ] `pkg/service/metadata_service_impl.go` — Orchestrates repo + producer + fetcher
- [ ] Feature flag injection into service
- [ ] All edge cases handled: duplicate POST, concurrent GET for same URL
- [ ] Unit tests covering all branches (mock everything)

**Test**: `go test -race ./pkg/service/...`

---

### Phase 5 — API Service (Day 3-4)

**Goal**: Fully working HTTP API. Handlers thin, all logic in service layer.

- [ ] `api/server/` — chi router, middleware stack, graceful shutdown
- [ ] Middleware: request ID, structured logging, OTel tracing, panic recovery
- [ ] `api/handlers/metadata_post.go` — POST /v1/metadata
- [ ] `api/handlers/metadata_get.go` — GET /v1/metadata
- [ ] `api/handlers/health.go` — GET /health, GET /ready
- [ ] Prometheus `/metrics` endpoint (feature-flagged)
- [ ] Swagger annotations + `make swagger` generation
- [ ] Handler unit tests (httptest)
- [ ] Wire everything in `api/main.go`

**Test**: `go test -race ./api/...` + manual curl against `docker-compose up`

---

### Phase 6 — Worker Service (Day 4)

**Goal**: Consumes Kafka, fetches, persists. Runs independently.

- [ ] `worker/consumer/consumer.go` — Kafka consumer loop with error handling
- [ ] Dead letter topic on max retries
- [ ] Manual offset commit after DB write
- [ ] Graceful shutdown (drain in-flight on SIGTERM)
- [ ] Metrics: messages consumed, fetch duration, errors
- [ ] Wire everything in `worker/main.go`
- [ ] Integration test: full pipeline via testcontainers

**Test**: `go test -race ./worker/...`

---

### Phase 7 — Integration & E2E Tests (Day 4-5)

**Goal**: Full flow verified with real infrastructure.

- [ ] `tests/integration/api_test.go` — POST → GET flow
- [ ] `tests/integration/worker_test.go` — Cache miss → worker → re-GET
- [ ] `docker-compose.test.yml` — Test profile
- [ ] GitHub Actions CI pipeline

**Test**: `make e2e` → all green

---

### Phase 8 — Documentation & Polish (Day 5)

**Goal**: Any engineer can clone and run in under 5 minutes.

- [ ] `README.md` — Full quick start, curl examples, config reference
- [ ] `docs/adr/` — ADR-001 through ADR-004
- [ ] `docs/runbook.md` — Ops tasks, common issues, scaling guide
- [ ] Review all Go doc comments on exported symbols
- [ ] `golangci-lint` clean
- [ ] `govulncheck` clean
- [ ] Final `docker-compose up` smoke test

---

## 17. Upgrade & Degradation Paths

### Scaling the Worker

Workers are stateless. Scale horizontally by increasing `replicas` in docker-compose or deploying more instances. Kafka partition count determines max parallelism — set to 6 initially.

### Replacing MongoDB

The `MetadataRepository` interface is the only contract. Add `postgres_repository.go`, inject it in `main.go`. Zero changes to service or handler code.

### Replacing Kafka

The `EventProducer` interface is the only contract. Add `rabbitmq_producer.go`, swap in `main.go`. Zero changes elsewhere.

### Adding Authentication

One middleware added to the chi router. No changes to handlers or business logic.

### Adding a Cache Layer (Redis)

Wrap the MongoDB repository with a `CachedRepository` that checks Redis first. Same interface, transparent to callers.

### Disabling Features Safely

Every non-core feature has a corresponding feature flag. To degrade gracefully:
- Disable page source storage: `FF_PAGE_SOURCE_STORAGE=false`
- Disable metrics: `FF_METRICS_ENABLED=false`
- Disable async worker: future flag to fall back to synchronous fetch on GET

---

## 18. Interviewer Impression Points

These are the decisions that signal senior engineering thinking:

| What You Did | Why It Impresses |
|---|---|
| Interface-driven design, all layers | "This engineer thinks about testability and replaceability from day one" |
| Manual DI in `main.go` | "They know what frameworks hide, and chose not to hide it" |
| Feature flags from day one | "They've shipped in production before — they know you can't just deploy" |
| ADRs in `docs/adr/` | "They document *why*, not just *what* — future maintainer-friendly" |
| `FROM scratch` Docker image | "They care about security and operational costs" |
| Manual Kafka offset commit | "They understand at-least-once delivery and idempotency" |
| `/health` vs `/ready` separation | "They understand Kubernetes liveness vs readiness probes" |
| SSRF protection on fetcher | "They think about security without being asked" |
| Sentinel errors + `errors.Is()` | "Idiomatic Go error handling — not just `if err != nil { return err }`" |
| `X-Request-ID` across services | "They've debugged distributed systems before" |
| `go test -race` in CI | "They know about Go's race detector and use it" |
| Dead letter topic for Kafka | "They've seen what happens when a bad message breaks a consumer" |

---

*This document is the single source of truth for implementation decisions. Update it as decisions evolve. Every deviation from this plan should be noted with a reason.*