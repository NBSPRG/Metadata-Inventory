# Runbook

## Local Startup

1. Create `.env` from the sample file with `make env`.
2. Start the stack with `docker compose up -d --build`.
3. Confirm:
   `api` readiness at `http://localhost:8080/ready`
   `worker` health at `http://localhost:9091/health`
   Prometheus at `http://localhost:9090`

## Kafka Failure Handling

- Worker retries message handling up to `KAFKA_MAX_RETRIES`.
- If all attempts fail, the original message is sent to `KAFKA_DLT_TOPIC`.
- The DLT message includes `original-topic`, `error`, `failed-at`, `retry-count`, and `max-retries` headers.
- If DLT publish fails, the consumer does not commit the offset, so Kafka can redeliver the message.

## Smoke Test

1. `POST /v1/metadata` with a valid URL.
2. `GET /v1/metadata?url=...` until the status becomes `ready`.
3. Check `/metrics` on both `api` and `worker`.
4. Stop Kafka temporarily and confirm `GET /ready` on the API returns `503`.
