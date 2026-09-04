# 00-spec — Consistency Hardening (SA criticals)

## Goal
Close the correctness/consistency bugs the Principal-SA review found at the
last-built seams so the purchase path is durable and the read-model can't
silently diverge. This is the **foundation** the PM cut-line (vouchers, etc.)
will later build on — no product features in this change. Done = every SA
Critical/High/Med below is fixed, each with a test that fails before and passes
after, and the golden purchase e2e still green.

## User stories (the ones that change decisions)
- **As the platform**, when OpenSearch rejects an index op, the event must NOT be
  lost: the consumer must retry then DLQ, never advance the offset past a failed
  record. (SA-C1/H5)
- **As a buyer**, if team-order crashes mid-checkout, my reserved stock must be
  released automatically (TTL sweep) and no orphan PENDING order remains. (SA-C2/M7)
- **As the platform**, a mock payment that settles must drive the order to PAID
  reliably (event-carried), never leave payment=PAID + order=PENDING. (SA-H3)
- **As the platform**, a retried checkout must not double-decrement stock. (SA-M6)

## Non-goals (explicit — do NOT scope-creep)
- No PM features: vouchers/promotions, seller reputation, category browse,
  variant enforcement — deferred to the next change.
- No real payments/wallet (policy §7 — stays mock).
- No full mTLS rollout. Zero-trust (SA-H4) is addressed by **ADR + a gitops
  NetworkPolicy only** in this change; service-identity mTLS is a follow-up.
- No new search relevance/ranking (SA product gap, not a correctness bug).

## Architecture decisions (SA-owned; agents follow, never re-decide)
- **AD1 — Consumer offset discipline.** Kafka consumers commit offsets ONLY after
  the handler succeeds. On handler error: bounded in-process retry (exp backoff,
  max N), then produce the record to a DLQ topic `<topic>.dlq` and only then
  advance. Never commit past an unprocessed record. *Why: today's at-most-once
  loses events on any transient sink error.*
- **AD2 — Read-model version guard.** Every read-model doc carries a monotonic
  `version` = source event's outbox seq / `occurred_at`. Updates apply only if
  incoming `version >= stored`; partial updates never upsert-create a tombstoned
  id. *Why: out-of-order/redelivered events currently resurrect deleted listings.*
- **AD3 — Saga durability + compensation context.** Reservation/saga state is
  persisted (Postgres in team-order) before external effects. Compensations run on
  a fresh `context.Background()` with their own deadline (never the failed request
  ctx), and failed releases are retried/parked, never `_, _ =` discarded. A
  reservation carries a TTL; team-domain sweeps and releases expired reservations.
  *Why: in-memory best-effort saga leaks stock and swallows compensation errors.*
- **AD4 — Payment→Order is event-carried.** team-payment does NOT call
  `order.UpdateOrderStatus` synchronously. On settle it writes a
  `platform.payment.v1.PaymentSettled` event via the SAME transactional-outbox
  pattern team-domain already uses; team-order consumes it idempotently and
  transitions the order. *Why: the current fire-and-forget RPC dual-writes.*
- **AD5 — Idempotent reservation.** The caller sets a stable
  `ReserveStockRequest.reservation_id` (per cart-item + attempt); team-domain makes
  reserve idempotent on it (re-reserve with same id = no-op). *Why: field exists in
  the contract but is never set → retries double-decrement.*
- **AD6 — Reuse existing patterns, no new deps.** Outbox + relayer + envelope
  already exist in team-domain (`internal/events/relayer.go`, `BuildListingChangedEnvelope`);
  DLQ/retry mirror team-domain's park-after-max-attempts. No new libraries.
  Delete the legacy inline `KafkaPublisher.produceEnvelope` (random event_id).

## Contracts (crossing task boundaries — code against these, not each other)
```proto
// platform-core/packages/proto/platform/payment/v1/payment.proto  (Wave 0 adds)
message PaymentSettled {
  string payment_id = 1;
  string order_id   = 2;
  string buyer_id   = 3;
  PaymentStatus status = 4;   // SETTLED | FAILED
  string occurred_at = 5;     // RFC3339; also the read-model version source
}
// Published on Kafka topic "payment.events", key = order_id,
// wrapped in platform.events.v1.EventEnvelope (event_id = outbox row id).
```
```
// Reservation idempotency (existing field, now REQUIRED from caller)
ReserveStockRequest.reservation_id  // stable per (cart_item, attempt)
// team-domain: reserve is a no-op if reservation_id already applied.

// DLQ topic convention (AD1): "<source-topic>.dlq", same EventEnvelope shape.
// Read-model version (AD2): OpenSearch doc field "version" (int64, from outbox seq).
```

## Conventions
- Tests: Go table tests next to code (`*_test.go`); each fix ships a test that
  reproduces the bug (red) then passes (green).
- Verify runs in **Docker** (`go`/`buf` not on host PATH): per service
  `docker compose -f docker-compose.services.yaml run --rm <svc> go test ./...`
  or the service's Dockerfile test stage. Proto: `make -C platform-core proto`
  runs buf inside its tooling.
- Never hand-edit `generated/` (the `guard-generated` hook blocks it) — regenerate.
- Commit format: `fix(<repo>): <what> [SA-<id>]`.
- Do NOT touch: any `team-frontend` code, any PM-feature surface, `plan/` (the old
  done plan).

## Locked
2026-09-03 — Scope: Hardening-only (SA Critical+High+Med + enabling proto/ADR +
doc-drift). Dir: plan-hardening/. Fan-out: Mode A (subagents), verify via Docker.
Decisions AD1–AD6 confirmed by user ("Hardening-only"). Assumptions locked:
(a) team-order owns saga-state Postgres tables; (b) payment.events is a NEW topic
via outbox; (c) zero-trust = ADR + gitops NetworkPolicy only, mTLS deferred;
(d) reservation_id already exists in proto (no breaking change), only PaymentSettled
is additive. Simpler-path pushback made & accepted: PM features excluded because
voucher/checkout depend on a durable saga that this change delivers first.
