# ADR 001: Async Fetch and Retry Semantics

## Status

Accepted

## Context

The service supports synchronous fetches for normal `POST` requests and asynchronous fetches for cache misses on `GET`. The worker also needs deterministic Kafka failure handling so failed jobs do not silently disappear.

## Decision

- `GET /v1/metadata` remains async on cache miss and publishes a Kafka fetch request.
- `POST /v1/metadata` stays synchronous by default, but `FF_ASYNC_FETCH_ONLY=true` forces it onto Kafka and returns a pending record.
- Worker message processing retries the handler up to `KAFKA_MAX_RETRIES`, then publishes the original payload to the dead-letter topic.
- Offsets are committed only after successful processing or successful DLT publication.

## Consequences

- Transient worker failures no longer go straight to the dead-letter topic.
- DLT outages do not lose work because offsets stay uncommitted.
- API readiness better reflects Kafka availability because the producer is checked explicitly.
