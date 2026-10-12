## Why

team-order is the phase 3 pilot for go-sdk v0.2.0 messaging. It uses every messaging piece: an outbox (`order_outbox_events`), the PaymentSettled consumer with a DLQ and dedupe, feature flags, and upstream clients. Its own copies have defects that the SDK fixes:

- **Lost record on DLQ failure.** `team-order/internal/consumer/payment.go:277-286` continues without committing when the DLQ produce fails, but a later commit then moves past that record.
- **Broken dedupe.** `team-order/internal/repository/processed_events.go:49-53` ignores the consumer name and swallows errors as "not processed". Dedupe is also not transactional with the state change.
- **Silent telemetry.** No panic recovery, and the OTEL settings are dead: `otelgrpc` is installed but no tracer provider is set up.
- **Always-on flags.** `KAFKA_ENABLED` and `OUTBOX_ENABLED` exist although every deploy sets them on (`platform-gitops/envs/services/team-order.yaml`, compose).

initiative: go-platform-sdk

## What Changes

- Vendor go-sdk at `go-sdk/v0.2.0` and move to the ADR-0016 layout:

  | From | To |
  |---|---|
  | `internal/handler` | `internal/rpc` |
  | `internal/service` | `internal/order` (cart, order, saga, returns, shipments, idempotency) |
  | `internal/repository` | `internal/store` |
  | `internal/consumer` | `internal/events` (payment handler) |
  | `internal/events` | stays (envelope builders) |
  | `internal/upstream` | stays (thin wrappers on SDK `upstream`) |
  | `internal/featureflags` | removed (SDK `flags`) |
  | `internal/bootstrap`, `internal/grpcserver`, `internal/interceptor` | removed (`internal/app` + SDK) |

  `cmd/resync-commits` keeps working.
- **Outbox:** SDK `outbox.Store` on the existing `order_outbox_events` table, plus the SDK `outbox.Relayer` as a lifecycle component. Row building stays in `internal/events`.
- **Consumer:** SDK `consume.Consumer` for `payment.events`, with DLQ `PAYMENT_EVENTS_DLQ_TOPIC`. SDK `consume.Dedupe` runs inside the same transaction as the PAID transition.
  - New migration `0008`: `processed_events` primary key becomes `(consumer, event_id)`, and existing rows get consumer `team-order.payment`.
- **Flags:** SDK `flags`. `FEATURE_FLAGS_ENABLED` stays, because it is a real toggle that promotion runs with false.
- **Upstream:** SDK `upstream.Dial`. The per-method service-principal scope map stays in `internal/upstream`.
- **Workers:** the relayer, consumer and reservation sweeper stop after the gRPC server (`lifecycle.StopAfter`).
- **Enable flags removed:** `KAFKA_ENABLED`, `OUTBOX_ENABLED`, `DATABASE_ENABLED` and `GRPC_REFLECTION_ENABLED`. They are also removed from the team-order compose block and from platform-gitops.
- **Database:** an unreachable database fails boot outside `ENV=local`. In local, the existing in-memory repositories are kept (the current RequireDurableStorage behaviour).

No RPC, proto or event payload changes.

## Non-goals

- Idempotency stays team-order code.
- Changing saga or business behaviour.
- Other services.

## Impact

- Repo: `team-order`, plus its compose block and `platform-gitops/{argocd/apps,envs/services}/team-order.yaml`.
- Schema: migration `0008` on `processed_events`. It is additive and safe on live data: the PK swap fills the consumer value for existing rows.
- Capability: `order-runtime` (new).

## Non-functional requirements

- No regression in about 151 existing tests, including the Postgres outbox, saga and CAS suites.
- Payment events are never lost: a failed DLQ produce stops the consumer without committing.

## Data classification

internal/personal, unchanged. Orders hold buyer ids and addresses, and no new data is collected.

## Threat model

| Asset | Threat (STRIDE) | Mitigation |
|---|---|---|
| Payment settlement | Tampering or loss: an offset is committed past an unhandled event | SDK consumer commits only after handling or a successful DLQ produce |
| Order state | Repudiation: double PAID from a replayed event | Transactional per-consumer dedupe |
| Errors | Information disclosure | `grpcx.Status` |
