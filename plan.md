# Metadata Inventory — Implementation Alignment Plan

Status: Active implementation baseline
Last Updated: 2026-04-22

This document replaces the original "greenfield" planning draft with an implementation-aligned plan that reflects what is currently in the repository.

## 1. Current Baseline

## 1.1 Runtime and Toolchain

- Module Go version: `1.25.1` ([go.mod](go.mod))
- CI Go version: `1.25.9` for all jobs ([.github/workflows/ci.yml](.github/workflows/ci.yml))
- API and worker are separate binaries with shared packages under `pkg/`.

## 1.2 Services in Compose

- `api`: HTTP endpoints and Swagger UI
- `worker`: Kafka consumer + metadata fetch pipeline
- `mongo`, `kafka`, `zookeeper`, `prometheus`

Stack definition: [docker-compose.yml](docker-compose.yml)

## 1.3 Implemented API Surface

- `POST /v1/metadata`
- `GET /v1/metadata?url=...`
- `GET /health`
- `GET /ready`
- `GET /metrics` (feature-flagged)
- `GET /swagger/*`

Route registration: [api/server/routes.go](api/server/routes.go)

## 2. Behavior Contract (As Implemented)

## 2.1 POST /v1/metadata

Source of truth:
- [api/handlers/metadata_post.go](api/handlers/metadata_post.go)
- [pkg/service/metadata_service_impl.go](pkg/service/metadata_service_impl.go)

Behavior:
- Validates JSON body and absolute http(s) URL
- If existing record is `ready`: returns `200`
- If existing record is `pending` or `fetching`: returns `202`
- If new URL:
  - `FF_ASYNC_FETCH_ONLY=true`: queues Kafka message and returns `202`
  - `FF_ASYNC_FETCH_ONLY=false`: fetches inline and returns `201` on success
- Inline fetch failure maps to `422 FETCH_FAILED`

## 2.2 GET /v1/metadata

Source of truth:
- [api/handlers/metadata_get.go](api/handlers/metadata_get.go)
- [pkg/service/metadata_service_impl.go](pkg/service/metadata_service_impl.go)

Behavior:
- `200` on cache hit (`ready`)
- `202` when record is in progress (`pending` or `fetching`)
- `202` after cache miss dispatches async fetch to Kafka

## 2.3 Health and Readiness

Source of truth:
- [api/handlers/health.go](api/handlers/health.go)

Behavior:
- `/health`: liveness only (`200`)
- `/ready`: checks MongoDB and Kafka producer ping (`200` when all OK, else `503`)

## 3. Data and Messaging

## 3.1 MongoDB Model

Source of truth:
- [pkg/db/models.go](pkg/db/models.go)
- [pkg/db/mongo_repository.go](pkg/db/mongo_repository.go)

Implemented fields include:
- URL, URL hash, status, schema version
- headers, cookies, page source, size, fetch duration
- retry count, fetched/created/updated timestamps

Implemented indexes:
- `url` unique sparse
- `url_hash` unique
- `status`
- `created_at` descending

## 3.2 Kafka Flow

Source of truth:
- [pkg/kafka/messages.go](pkg/kafka/messages.go)
- [pkg/kafka/consumer.go](pkg/kafka/consumer.go)
- [worker/consumer/consumer.go](worker/consumer/consumer.go)

Implemented semantics:
- Message carries version, URL, requested timestamp, request ID, source
- Manual offset commit after success or successful DLT routing
- Retries with bounded backoff
- DLT routing after max retries

## 4. Feature Flags (Implemented)

Source of truth:
- [pkg/featureflags/flags.go](pkg/featureflags/flags.go)
- [pkg/config/config.go](pkg/config/config.go)

Implemented flags:
- `FF_RATE_LIMIT_ENABLED`
- `FF_CIRCUIT_BREAKER_ENABLED` (reserved; currently no breaker wiring)
- `FF_PAGE_SOURCE_STORAGE`
- `FF_ASYNC_FETCH_ONLY`
- `FF_METRICS_ENABLED`
- `FF_TRACING_ENABLED`

## 5. Observability and Middleware

Source of truth:
- [api/server/server.go](api/server/server.go)
- [pkg/middleware](pkg/middleware)
- [pkg/observability](pkg/observability)

Implemented:
- Request ID middleware
- Panic recovery middleware
- Structured logging middleware
- Real IP extraction
- OTel tracing middleware (no-op when tracing disabled)
- Prometheus metrics middleware and endpoint (flag-gated)

## 6. Testing and CI (Current)

## 6.1 Test Inventory

- Unit and package tests across `api/`, `pkg/`, `worker/`
- Non-docker E2E tests in [api/e2e_test.go](api/e2e_test.go)
- Docker full-stack E2E in [api/e2e_docker_test.go](api/e2e_docker_test.go)

## 6.2 CI Jobs

Source of truth:
- [.github/workflows/ci.yml](.github/workflows/ci.yml)

Pipeline:
- `lint`: install and run `golangci-lint` from source
- `security`: `govulncheck` + `gitleaks`
- `build-and-unit`: `go vet`, `go build`, `go test -race`
- `coverage`: enforce `COVERAGE_MIN=25`
- `e2e-docker`: docker compose stack + tagged E2E test

Important CI behavior:
- E2E job appends `FF_ASYNC_FETCH_ONLY=true` to `.env` to make POST flow deterministic.

## 7. Documentation Corrections from Original Draft

The earlier planning document included future-state items that are not present in this repository (for example: `go.work`, `testcontainers-go` integration suite, `docker-compose.test.yml`, and several ADR filenames).

This updated plan now tracks only implemented behavior and explicit next steps.

## 8. Forward Plan (Prioritized)

1. Align module toolchain with CI by updating `go.mod` to patched Go toolchain policy.
2. Raise coverage gate incrementally (for example, `25 -> 30 -> 40`) with targeted tests.
3. Either implement circuit breaker wiring or remove `FF_CIRCUIT_BREAKER_ENABLED` until used.
4. Add regression tests for POST status transitions in sync vs async-only modes.
5. Expand runbook with failure playbooks for Kafka DLT growth and readiness degradation.

## 9. Definition of Done for This Phase

- Docs reflect actual API and CI behavior.
- No references to non-existent files or workflows.
- README and plan remain consistent after each CI policy change.

