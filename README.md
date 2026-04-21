# Metadata Inventory

HTTP metadata capture service with an API, Kafka-backed async processing, MongoDB persistence, and Prometheus metrics.

## Services

- `api`: exposes `POST /v1/metadata`, `GET /v1/metadata`, `/health`, `/ready`, and `/metrics`
- `worker`: consumes Kafka fetch jobs, stores results in MongoDB, and exposes `/metrics` plus `/health` on `WORKER_METRICS_PORT`
- `mongo`, `kafka`, `zookeeper`, `prometheus`: local development infrastructure

## Quick Start

1. Create a local env file with `make env`.
2. Start the stack with `docker compose up -d --build`.
3. Confirm health:
   `http://localhost:8080/health`
   `http://localhost:8080/ready`
   `http://localhost:9090`
4. Submit a URL:
   `curl -X POST http://localhost:8080/v1/metadata -H "Content-Type: application/json" -d "{\"url\":\"https://example.com\"}"`

## Runtime Notes

- Worker retries failed Kafka jobs up to `KAFKA_MAX_RETRIES` before publishing to the dead-letter topic.
- Offsets are committed only after success or a confirmed DLT write.
- API readiness now checks MongoDB and Kafka producer connectivity.
- `FF_ASYNC_FETCH_ONLY=true` forces `POST /v1/metadata` onto Kafka and returns a pending record.

## Key Env Vars

- `HTTP_PORT`: API port, default `8080`
- `WORKER_METRICS_PORT`: worker metrics port, default `9091`
- `KAFKA_BROKERS`: comma-separated broker list
- `KAFKA_TOPIC`: fetch request topic
- `KAFKA_DLT_TOPIC`: dead-letter topic
- `KAFKA_MAX_RETRIES`: retry count before DLT
- `FF_ASYNC_FETCH_ONLY`: queue POST requests instead of fetching inline
- `FF_PAGE_SOURCE_STORAGE`: persist fetched HTML body

## Testing

- `go test ./...`
- `go test -race ./...`

## API Docs

- Swagger UI: `http://localhost:8080/swagger/index.html`
- Regenerate docs: `make swagger`

## Docs

- [Runbook](docs/runbook.md)
- [ADR: Async Fetch and Retry Semantics](docs/adr/001-async-fetch-and-retry.md)
