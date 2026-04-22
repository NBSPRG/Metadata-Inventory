# Metadata Inventory

Metadata Inventory is a Go-based HTTP metadata capture system with an API service, Kafka-backed async worker, MongoDB persistence, and Prometheus metrics.

## Services

- `api`: `POST /v1/metadata`, `GET /v1/metadata`, `/health`, `/ready`, `/metrics`, `/swagger/*`
- `worker`: Kafka consumer that fetches URL metadata and persists it to MongoDB
- `mongo`, `kafka`, `zookeeper`, `prometheus`: local infrastructure via Docker Compose

## Quick Start

This project uses `make` to simplify execution.

1. Install dependencies (Ubuntu/Debian only, requires `sudo`):
   ```bash
   make install-deps
   ```
2. Create local environment configs:
   ```bash
   make env
   ```
3. Start the entire stack via Docker:
   ```bash
   make up
   ```
4. Check health endpoints to verify everything is running:
   - `http://localhost:8080/health`
   - `http://localhost:8080/ready`
5. Submit a URL for fetching:
   ```bash
   curl -X POST http://localhost:8080/v1/metadata -H "Content-Type: application/json" -d "{\"url\":\"https://example.com\"}"
   ```

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

The project uses `make` targets to run tests.

- **Run all unit/integration tests:**
  ```bash
  make test
  ```
- **Run the Full E2E Test Suite against Docker:**
  Automatically spins up containers, assertions, and tears down safely.
  *(Ensure `make install-deps` has been executed if on a clean VM)*
  ```bash
  make test-e2e-docker
  ```
- **Review Code Coverage:**
  ```bash
  make test-cover
  ```

## CI Overview

Pipeline file: [.github/workflows/ci.yml](.github/workflows/ci.yml)

- Go runtime pinned to `1.25.9` in CI
- Lint: installs and runs `golangci-lint` from source
- Security: `govulncheck` and `gitleaks`
- Coverage gate: minimum `25%`
- Docker E2E sets `FF_ASYNC_FETCH_ONLY=true` before stack startup

## Deployment Strategy (CD)

While a full Continuous Deployment pipeline is not implemented here to avoid requiring specific cloud credentials, the project is structured to be immediately deployable to modern container orchestration platforms (like AWS ECS/EKS or Google Cloud Run/GKE).

In a production scenario, the deployment pipeline would look like this:
1. **Build & Publish**: The GitHub Actions CI pipeline would be extended to run `docker build` and `docker push` for both `api` and `worker` images, pushing them to a secure registry (e.g., AWS ECR or Docker Hub) upon merging to `main`.
2. **Infrastructure as Code**: Terraform or Pulumi would be used to provision the required cloud resources (managed MongoDB Atlas, managed Kafka/MSK, and the container runtime).
3. **Continuous Deployment**: A tool like ArgoCD (for Kubernetes) or an ECS rolling update step in GitHub Actions would deploy the new image tags automatically.
4. **Environment Variables**: Feature flags and configurations (like `FF_ASYNC_FETCH_ONLY`, database URIs, etc.) would be injected securely via AWS Secrets Manager or Kubernetes Secrets.

## API Docs

- Swagger UI: `http://localhost:8080/swagger/index.html`
- Regenerate: `make swagger`

## Documentation

- [Implementation Alignment Plan](plan.md)
- [Runbook](docs/runbook.md)
- [ADR: Async Fetch and Retry Semantics](docs/adr/001-async-fetch-and-retry.md)
