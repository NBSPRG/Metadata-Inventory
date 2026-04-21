# Metadata Inventory

Metadata Inventory is a Go-based HTTP metadata capture system with an API service, Kafka-backed async worker, MongoDB persistence, and Prometheus metrics.

## Services

- `api`: `POST /v1/metadata`, `GET /v1/metadata`, `/health`, `/ready`, `/metrics`, `/swagger/*`
- `worker`: Kafka consumer that fetches URL metadata and persists it to MongoDB
- `mongo`, `kafka`, `zookeeper`, `prometheus`: local infrastructure via Docker Compose

## Quick Start

1. Create local env file:
   `make env`
2. Start stack:
   `docker compose up -d --build`
3. Verify:
   - `http://localhost:8080/health`
   - `http://localhost:8080/ready`
   - `http://localhost:9090`
4. Submit URL:
   `curl -X POST http://localhost:8080/v1/metadata -H "Content-Type: application/json" -d "{\"url\":\"https://example.com\"}"`

## API Behavior

### POST /v1/metadata

- `201`: new URL fetched inline successfully (`FF_ASYNC_FETCH_ONLY=false`)
- `200`: URL already available (`ready`)
- `202`: URL already `pending/fetching` or async-only queue mode enabled
- `422`: fetch failed in inline mode

### GET /v1/metadata?url=...

- `200`: cache hit (`ready`)
- `202`: in progress or cache miss queued to Kafka

### Health Endpoints

- `/health`: liveness
- `/ready`: readiness (MongoDB + Kafka producer connectivity)

## Runtime Notes

- Worker retries failed messages up to `KAFKA_MAX_RETRIES`, then routes to DLT.
- Kafka offsets are committed only after success or successful DLT write.
- `FF_ASYNC_FETCH_ONLY=true` forces POST to queue mode for deterministic async flow.

## Key Env Vars

- `HTTP_PORT` (default `8080`)
- `WORKER_METRICS_PORT` (default `9091`)
- `MONGO_URI`, `MONGO_DB`
- `KAFKA_BROKERS`, `KAFKA_TOPIC`, `KAFKA_GROUP_ID`, `KAFKA_DLT_TOPIC`, `KAFKA_MAX_RETRIES`
- `FF_ASYNC_FETCH_ONLY`, `FF_PAGE_SOURCE_STORAGE`, `FF_METRICS_ENABLED`, `FF_TRACING_ENABLED`

See full defaults in [.env.example](.env.example).

## Testing

- Unit/package tests: `go test -race -count=1 ./...`
- Docker E2E: `go test -tags=e2e_docker -count=1 -v ./api -run TestE2E_DockerFullStack`

## CI Overview

Pipeline file: [.github/workflows/ci.yml](.github/workflows/ci.yml)

- Go runtime pinned to `1.25.9` in CI
- Lint: installs and runs `golangci-lint` from source
- Security: `govulncheck` and `gitleaks`
- Coverage gate: minimum `25%`
- Docker E2E sets `FF_ASYNC_FETCH_ONLY=true` before stack startup

## API Docs

- Swagger UI: `http://localhost:8080/swagger/index.html`
- Regenerate: `make swagger`

## Documentation

- [Implementation Alignment Plan](plan.md)
- [Runbook](docs/runbook.md)
- [ADR: Async Fetch and Retry Semantics](docs/adr/001-async-fetch-and-retry.md)
