## Why

Phase 3 of the go-platform-sdk initiative. Seven services carry their own Kafka, outbox and consumer code, and the copies have diverged (survey 2026-10-12):

**Outbox**
- There are three outbox families.
- Domain, identity and payment, and order and chat, share the same columns but use different claim, retry and park SQL.
- Engagement has no attempts, no backoff and no parking, so one bad message blocks its queue.

**Consumers**
- There are two consumer loops with different retry and commit rules.
- team-order commits past a record whose DLQ produce failed (`team-order/internal/consumer/payment.go:277-286`).
- team-order's `processed_events` lookup ignores the consumer and swallows errors (`team-order/internal/repository/processed_events.go:49-53`).
- Dedupe is not transactional with the side effect.

**Producers**
- No producer propagates `traceparent`. Only promotion sets it in the envelope.
- Kafka config is read in three different ways.

**Other copies**
- Feature flags (order and promotion) and upstream dial helpers (five services) are copied.

**Pilot findings**
The pilots (#14, #15) also left these follow-ups:
- the sync script needs `generated/`;
- the synced lint config keeps the platform-core goimports prefix;
- `make docker` ignores `GRPC_PORT`;
- `lifecycle` cannot stop workers after the server (team-domain wrote `afterServer`).

initiative: go-platform-sdk

## What Changes

New go-sdk packages, released as `go-sdk/v0.2.0`:

| Package | What it provides |
|---|---|
| `kafkax` | franz-go producer built from `config.Kafka` (`KAFKA_BROKERS`), with OTel (kotel) and `traceparent` / `x-request-id` record headers from ctx. Exposes `Publish(ctx, topic, key, value)` and `Close`. |
| `outbox` | Postgres outbox store for the shared column set: `event_id, aggregate_type, aggregate_id, event_type, payload, request_id, status, attempts, available_at, locked_until, error, published_at, created_at`. The **table name is a parameter**, so existing tables are kept. Provides `EnqueueTx`, a SKIP LOCKED claim, publish/retry/park, and a `Relayer` lifecycle component configured by `config.Outbox` (`OUTBOX_POLL_INTERVAL`, `OUTBOX_BATCH_SIZE`, `OUTBOX_CLAIM_LOCK_SECONDS`, `OUTBOX_MAX_ATTEMPTS`). |
| `consume` | franz-go group consumer with manual commit. Handler errors retry with exponential backoff; `consume.Permanent(err)` skips retries. Exhausted records go to a configured DLQ topic, then the offset is committed. **If the DLQ produce fails, the loop returns an error and the offset is not committed.** |
| `consume.Dedupe` | `Seen(ctx, tx, consumer, eventID) (bool, error)` on a `processed_events (consumer, event_id)` table. It runs inside the handler's transaction. |
| `flags` | OpenFeature + Flipt evaluator promoted from team-order. `FEATURE_FLAGS_ENABLED` stays, because promotion runs with it set to false in gitops. |
| `upstream` | `Dial(addr)` with otelgrpc and a deadline helper (`Bounded`). Interceptors forward request-id and principal, or replace the principal with a configured service principal. |

Also in v0.2.0:
- **`lifecycle.StopAfter(primary, dependents...)`**: dependents keep running until the primary component has returned, then they are cancelled.
- **Tooling:**
  - `sync-go-sdk.sh` runs `buf generate` when `generated/` is missing, and rewrites goimports `local-prefixes` in the synced `.golangci.yml` to the service's module path;
  - `make docker` passes `GRPC_PORT`;
  - the template Dockerfile has no `GRPC_PORT` default.
- **Not in the SDK:** idempotency stays in team-order, because it is tied to `order_sagas` and only one service uses it.

**BREAKING:** none for existing consumers. The new packages are additive. `lifecycle` gains a function.

## Non-goals

- Migrating any service. Services migrate in their own changes (order is the phase 3 pilot).
- Changing an outbox table name or schema in any service.
- Moving engagement's outbox onto the shared schema. That happens in engagement's migration.
- Schema registries, exactly-once semantics, and the River vs RabbitMQ task-store decision.

## Impact

- Repos: `platform-core` only (`packages/go-sdk`, `tools/sync-go-sdk.sh`, `make/go-service.mk`, `templates/go-service/Dockerfile`, ADR-0016 text).
- New SDK dependencies:
  - github.com/twmb/franz-go and its kotel plugin
  - github.com/open-feature/go-sdk with the Flipt provider

## Non-functional requirements

- The relayer publishes at least once and in claim order per batch, and a poison row is parked after `OUTBOX_MAX_ATTEMPTS`.
- The consumer never commits past a record that was neither handled nor dead-lettered.

## Data classification

internal. Events carry principal ids and request ids already present today. Nothing new is collected.

## Threat model

| Asset | Threat (STRIDE) | Mitigation |
|---|---|---|
| Event stream | Tampering or loss: an offset is committed past an undelivered record | The commit only happens after the handler succeeded or the DLQ produce succeeded |
| Outbox | Denial of service: a poison row blocks the queue | Attempts, backoff, then park |
| Trace and request headers | Information disclosure | Headers carry ids only, never payload or tokens |
