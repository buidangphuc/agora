# ADR-0002 — Async broker

**Status:** Accepted (Kafka + RabbitMQ) · **Date:** 2026-08-30

## Context

Some flows are async: event-driven read-models (search indexing), cross-service
notifications, outbox relay, and background jobs. The platform needs shared broker
infra. These are two different problems with different semantics — a replayable
event log vs. a work queue — so one broker is the wrong tool for both.

## Decision

Run **two brokers, split by role**:

- **Kafka — event streaming.** State-change events between services (a listing was
  created/updated → a search read-model reindexes). Replayable log, consumer groups,
  many independent consumers, event-carried state. **Local runtime = Redpanda**
  (Kafka-API compatible, no JVM/ZooKeeper); prod can be Apache Kafka — same client.
- **RabbitMQ — task / work queues.** Command-style jobs with one logical consumer,
  at-least-once delivery, retries + DLQ (embedding jobs, notifications, webhooks).
  **Prod path → SQS** (team-ai already has a pluggable queue with `rabbitmq`/`sqs`
  backends behind one `QueueGateway` port).

### Conventions

- **Event topics:** `<domain>.events` (e.g. `listing.events`). **Message key =
  aggregate id** (listing id) so per-entity ordering holds within a partition.
- **Envelope:** every event is a `platform.events.v1.EventEnvelope` — `type`
  (fully-qualified proto name), `payload` (serialized domain message), plus
  `principal` (ADR-0003) and `traceparent` / `request_id` (ADR-0004) for authz +
  correlation across the async hop. Proto-first, so buf breaking-checks apply.
- **Consumers are idempotent** (upsert/delete by id) so a topic can be replayed to
  rebuild a read-model from scratch.
- **Producing an event** is done from the write path today (directly after the DB
  write). Production hardening = **transactional outbox** (team-ai already ships an
  `outbox/` store; the relay process is future work) so the event and the DB commit
  can't diverge.

## Consequences

- `infra/docker-compose.yaml` runs `redpanda`, `rabbitmq`, and `opensearch` (the
  first search read-model, ADR-0005). All start with `make dev`.
- Choosing the wrong broker for a flow is a review smell: events → Kafka, jobs →
  RabbitMQ. Don't push replayable state changes through RabbitMQ or fan-out jobs
  through Kafka.
- Redpanda→Kafka and RabbitMQ→SQS are config/runtime swaps, not code rewrites.
