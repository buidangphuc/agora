## Why

team-payment still hand-rolls what go-sdk v1.0.2 provides: config binding, logging, tracing, gRPC server and interceptors (`internal/interceptor`, `internal/grpcserver`), health, lifecycle, the outbox relayer (`internal/events/relayer.go`, `internal/repository/outbox*.go`), the Kafka consumer loop (`internal/consumer`, `internal/bootstrap`) and the audit client (`internal/upstream/audit.go`). It also carries dependency switches and runtime fallbacks the user ruled out on 2026-10-22: `DATABASE_ENABLED` (with in-memory repositories wired in `cmd/server/main.go` when the pool is nil), `KAFKA_ENABLED` and `PAYMENT_SETTLEMENT_CONSUMER_ENABLED` (without them neither the relayer nor the settlement consumer runs and sellers are never credited), a settle path that "degrades to a status-only update" without a transaction writer, and `MOCK_PAYMENTS`, a mock switch that is not on the shared mock-data mechanism.

The liveness probe also depends on the single overall health status, so a Postgres or Kafka outage restarts the pod in a loop; the SDK separates readiness (overall) from liveness (`liveness`).

Plan: `plans/go-sdk-migration/PLAN.md` (Step 2, payment: 37 commits on main since the first migration; main's behaviour wins over `origin/feat/go-sdk-v1-e2e`, which is reference only). Builds on archived SDK changes; SDK requirements are in `openspec/specs/go-platform-sdk`.

initiative: go-sdk-migration

## What Changes

- Module: require `github.com/buidangphuc/agora/platform-core/packages/go-sdk@v1.0.2`; no `replace`, no `third_party`. The Makefile includes `../platform-core/make/go-service.mk` and lints with `../platform-core/packages/go-sdk/.golangci.yml`; the service holds no copy of either. `golangci-lint` is pinned as a `go tool`, as in the SDK `go.mod`.
- Layout (ADR-0016): `cmd/server/main.go` -> `internal/app.Run(ctx, settings)`; `internal/config`; `internal/rpc` (was `handler` + `interceptor` + `grpcserver`); `internal/payment` (was `service`); `internal/settlement` (was `consumer` + `bootstrap`); `internal/store` (was `repository`); `internal/events` keeps only the envelope builder; `internal/upstream`; `internal/pgtest` replaced by SDK testkit. Packages `bootstrap`, `interceptor`, `grpcserver` and the relayer and outbox store files are deleted.
- **BREAKING (config)**: `ENV` is required (no `local` default); `SHUTDOWN_GRACE_SECONDS` becomes `SHUTDOWN_GRACE` (Go duration, `10s`); `GRPC_REFLECTION_ENABLED` defaults to false and is refused in a strict ENV; `UPSTREAM_CALL_TIMEOUT_SECONDS` is removed (audit writes use the SDK 2 s bound); `UPSTREAM_ORDER_ADDR` and `UPSTREAM_AUDIT_ADDR` are required in every ENV (no fallback mode); `KAFKA_BROKERS` and `LOG_KEY_FP_SECRET` are required in a strict ENV.
- **REMOVED**: `DATABASE_ENABLED`, `KAFKA_ENABLED`, `PAYMENT_SETTLEMENT_CONSUMER_ENABLED`, `MOCK_PAYMENTS`; the in-memory repository wiring in `main`; the status-only settle fallback; the nil-audit-writer branch and the empty `UPSTREAM_AUDIT_ADDR` local mode. In-memory stores remain only as test fakes in `internal/store/fake`, imported by no non-test code.
- **MOCK DATA**: `MOCK_PAYMENTS=true` becomes `MOCK_DATA=payment-provider` (`pkg/mockdata`). `ProcessMockPayment` runs the real path (transaction row, same-transaction `PaymentSettled` outbox row, ledger credit through `OrderPaidEvent`); only the payment-provider adapter outcome is the fixture one. Refused in a strict ENV before any other validation.
- **Outbox**: the SDK `outbox.Store` and `outbox.Relayer` replace the hand-rolled ones over the existing table `payment_outbox_events`; migrations `0008`-`0017` upgrade it to the SDK v1 shape (`seq`, `topic`, `traceparent`) in order-preserving steps and requeue rows parked as `failed`. Per-order ordering now follows `seq`. Published rows are purged after 7 days by `Store.Purge` on a schedule in the service. The first rollout scales old pods to 0 before new pods start.
- **Consumer**: the order.events settlement consumer (OrderPaidEvent credit, OrderCancelled refund or void, ReturnRefunded refund) runs on `consume.Consumer` with `kafkax.NewConsumerClient`, SDK retry and DLQ (`order.events.payment-settlement.dlq`), and logs `key_fp` HMAC fingerprints, never keys. Business idempotency (ledger unique keys) is unchanged; no `processed_events` table is added.
- **Audit-first refunds**: admin `RefundPayment` writes the audit event through `auditx.Gate` before moving money; failure returns `Unavailable` with message `audit trail unavailable` (was `audit unavailable`). The audit connection uses `auditx.Dial` (service principal `service-team-payment`, scope `audit.write`); the order connection uses `upstream.NewClient` with `upstream.AsService("service-team-payment", "order.read")`.
- **Migrations**: `server migrate up` subcommand (SDK `pkg/migrate`, embedded); the service never migrates at boot; compose `team-payment-migrate` and a gitops PreSync Job run it (deploy track).
- **Health**: readiness is the overall status (the `postgres` check only; Kafka is not a readiness check); liveness is the `liveness` service. The chart sets `probes.livenessService: liveness` (deploy track).
- **Errors**: `grpcx.Status` with a service error table; unique-key violation 23505 maps to `AlreadyExists`; `context.Canceled` and `DeadlineExceeded` map to their gRPC codes. Unexpected errors return `Internal` with "internal error" (existing no-echo rule kept).
- **Pagination**: `ListLedgerEntries` uses `pagination.Clamp`, `EncodeOffset`, `DecodeOffset`; the opaque offset cursor format and the default 20 / max 100 page sizes are unchanged.
- **Not used**: `flags`, `idempotency`, `jwtx`, `resilience` (the service has no Flipt flags, no client idempotency keys beyond `refund_id`, no token verification, no outbound retry policy today).
- Specs: ADDED capability `payment-service-runtime`; MODIFIED `payment-access-control` (mock opt-in), `admin-refund-audit` (audit unavailable behaviour) and `admin-refund-audit` (no nil-writer mode). The shared `deploy-runtime` requirement is MODIFIED only by deploy-go-sdk-v1 (see Deploy track in tasks.md).

## Non-functional requirements

- Availability: the service is ready while the Postgres check passes (a broker outage does not make it unready); liveness stays SERVING during a dependency outage (no restart loop). Boot fails fast (non-zero exit) when Postgres is unreachable or `DATABASE_URL` is empty, in every ENV.
- Latency: a settled payment's `PaymentSettled` row is claimed within one `OUTBOX_POLL_INTERVAL` (1 s default); steady-state publish p95 <= 2 s. Admin refund: audit write bounded at 2 s, call returns `Unavailable` within 3 s when audit is down. Existing RPC latency budgets are unchanged.
- Throughput and limits: `OUTBOX_BATCH_SIZE` 100, `OUTBOX_CLAIM_LOCK` 60 s, consumer `MaxPollRecords` 500 and `MaxAttempts` 5; `ListLedgerEntries` page size default 20, max 100.
- Shutdown: health drains, `SHUTDOWN_DRAIN_DELAY` passes, the gRPC server stops within `SHUTDOWN_GRACE` (10 s), then the relayer and consumer stop, then Kafka clients and the pool close.
- Delivery: at-least-once both ways; credits, refunds and settle stay exactly-once in effect through the existing unique keys.
- Deploy: the outbox upgrade runs with `CREATE INDEX CONCURRENTLY` (no write lock beyond the brief `SHARE ROW EXCLUSIVE` in step 0010); old pods are scaled to 0 before `0008` and before the new pods start.
- Tests: Docker-backed suites must run; a skip is a gate failure.

## Data classification

| Field | Class | Purpose | Basis | Retention |
|---|---|---|---|---|
| `buyer_id`, `seller_id`, `actor_id` (user ids) | pseudonymous personal identifier | authorize and attribute payments, wallet ledger, audit events | legitimate interest (operating the marketplace payment ledger) | database rows unchanged by this change; logs follow the platform log retention |
| `order_id`, `payment id`, `refund_id`, amounts | transaction data | payment, refund, settlement, audit metadata | legitimate interest | unchanged by this change |
| `bank_code`, `account_number`, `account_name` (payout request) | sensitive financial | mock payout record (no real rail, root AGENTS.md section 7) | legitimate interest | unchanged by this change; never logged, never in audit or outbox events |
| Kafka record key (`order_id`) in logs | derived identifier | diagnose consumer failures | legitimate interest | only `key_fp` (HMAC-SHA256 with `LOG_KEY_FP_SECRET`) is logged; platform log retention |
| `x-request-id`, traceparent in outbox rows | technical id | trace continuity | legitimate interest | purged with published outbox rows |

No new personal field is collected. Per the user rule no DPO sign-off is required: legitimate interest, platform log retention.

## Threat model

- Assets: payment transactions, seller wallet ledger and payouts, `payment_outbox_events`, the audit trail of admin refunds, `LOG_KEY_FP_SECRET`.
- Abuse cases and controls:
  - Mock settlement in a shared environment: `MOCK_DATA` non-empty with a strict ENV refuses boot before validation and names `MOCK_DATA`; an unknown adapter name refuses boot.
  - Debug surface: gRPC reflection is off by default and refused in a strict ENV.
  - Silent local defaults in a deployed environment: `ENV` has no default; `UPSTREAM_ORDER_ADDR` and `UPSTREAM_AUDIT_ADDR` are required in every ENV; strict ENV also requires `KAFKA_BROKERS` and `LOG_KEY_FP_SECRET`.
  - Admin refund without audit: `auditx.Gate` writes the event first and fails closed with `Unavailable`; no refund row, no ledger entry.
  - Poison or replayed order events: permanent errors park at once on the DLQ; redelivery is a no-op through the ledger unique keys.
  - Restart loop on dependency outage: liveness is independent of checks.
  - Identifier leakage in logs: consumer logs `key_fp` only; handler errors never echo driver text.
  - Outbox upgrade reorders events: step 0009 numbers old rows by `(created_at, event_id)`; ordering is tested.
- Authorization rule (unchanged): every RPC needs a principal (anonymous rejected with `Unauthenticated`); `CreatePayment` and `ProcessMockPayment` require the transaction's buyer or admin; `RefundPayment` requires the order's seller or scope `admin`; wallet RPCs follow `sellerAccess` (owner, admin read-only); payouts need `listing.write` and are owner-only.
