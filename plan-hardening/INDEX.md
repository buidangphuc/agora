# Plan Index — Consistency Hardening (SA criticals)

Single source of truth for orchestration. Update **Status** after every task event.
Status: `todo | running | review | done | failed`. Fan-out: Mode A (subagents).
Verify in **Docker** (`go`/`buf` not on host PATH).

## Waves & tasks

| Wave | Task file | Repo (writes) | SA finding | Verify | Status |
|---|---|---|---|---|---|
| 0 | wave0-task1-proto-adr-docs | platform-core (proto+ADR) + AGENTS.md | enabling | `make -C platform-core lint-proto`; ADRs render | review (ADR+doc done; proto edited — needs Docker buf lint+regen) |
| 1 | wave1-task1-search-exactly-once | team-search | C1 + H5 | `go test ./...` (Docker) | done (consumer+index green) |
| 1 | wave1-task2-domain-reservation | team-domain | M6 + C2(domain) + Low | `go test ./...` (Docker) | done (svc+repo green; handler/bootstrap wiring &rarr; W3) |
| 1 | wave1-task3-payment-event | team-payment | H3 (producer) | `go test ./...` (Docker) | done (outbox green; main.go wiring &rarr; W3) |
| 1 | wave1-task4-gateway-limiter | team-gateway | Med (limiter) | `go test ./...` (Docker) | done (green; cockpit.go delta = baseline, not agent) |
| 2 | wave2-task1-order-saga-durable | team-order | C2 + H3(consumer) + M6 + M7 + M8 | `go test ./...` (Docker) | done (unit-green + review-passed; e2e proven in W3) |
| 3A | wave3 wiring (4 repos) | payment/domain/search/order | wiring+ForceFailSaga+testfix | go test (Docker) x4 | done (all green) |
| 3B | wave3 infra+e2e | compose/gitops/e2e | topics/tables/netpol/ACL + purchase-saga e2e | done (e2e PASS; gates clean) |

## Dependency notes
- **Wave 0 solo** — proto `PaymentSettled` + ADR-0007..0010 + doc-drift. Unblocks
  W1-T3 (payment event) and W2-T1 (order consume). No code depends on the ADRs.
- **Wave 1 = 4 disjoint repos**, fully parallel (polyrepo → structurally disjoint).
  W1-T3 reads the W0 proto; W1-T1/T2/T4 are independent.
- **Wave 2 solo (team-order)** — the heavy seam; depends on W0 (proto) + W1-T2
  (idempotent reserve contract) + W1-T3 (PaymentSettled producer).
- **Wave 3 integration** — netpol, compose/env (payment.events, DLQ topics, saga
  tables), regen check, e2e, docs, gates.

## Review gate (Step 5.3 — reviewer ≠ implementer)
Every task reviewed by a DIFFERENT agent using the packet's Role rubric AND the
project reviewer subagents:
- any task touching service boundaries / brokers / proto → **contract-boundary-reviewer**
- any task touching auth / principal / scopes → **auth-scope-reviewer**
Verdict `done | failed:<reason>`. 1 retry max, then escalate.

## Merge order
By wave; within a wave, arbitrary (disjoint repos). Generated code (`generated/`,
buf output) is in NO write-set — regenerated per repo as part of its task, never merged.

## Verify recipe (Docker)
```bash
# per Go service
docker compose -f docker-compose.services.yaml run --rm <svc> go test ./...
# proto
make -C platform-core lint-proto && make -C platform-core proto
# full e2e (Wave 3)
make -C platform-e2e up && pytest -k purchase_saga
```

## Known pre-existing drift (fix in Wave 3, not caused by this plan)
- `team-search/internal/handler/search_test.go` (mtime Sep 1, baseline) references a
  renamed API: `SuggestListings*` → proto is now `Suggest`/`SuggestRequest`, and
  `interceptor.ContextWithPrincipal` → `contextWithPrincipal`. Stale test won't
  compile → team-search full `go test ./...` is red independent of W1-T1. Wave 3
  integration: rename to the current API (mechanical) so the suite is green.

## W3 wiring queue (from Wave-1 STOP reports — main.go/upstream were out of scope)
- team-payment: wire `PgTxWriter` (SettleTx) + `Relayer` into `cmd/server/main.go`
  so settle actually publishes to `payment.events` (today: safe status-only fallback).
- team-payment: `OrderClient.UpdateOrderStatus` now unused — remove the dead upstream call in W3.
- team-domain: wire `handler/listing.go` ReserveStock → `ReserveStockIdempotent(req.GetReservationId(), ...)`; start `ReservationSweeper.Run` in `internal/bootstrap`.
- team-domain: fix pre-existing stale `internal/handler/listing_test.go` (ContextWithPrincipal, "published" string→ListingStatus enum, PageSize) — mechanical, mirrors the search one.

## Review-gate findings (W2 team-order) → W3 actions
- [auth CLEAN] but FIX in W3: gate `team-order/internal/handler/order.go` `ForceFailSaga` + `GetSagaState` with `RequirePrincipal` + admin/owner check (pre-existing hole, now amplified: unauth caller can cancel+ReleaseStock any order).
- [auth QUESTION] W3 infra: Kafka ACL — only team-payment may produce to `payment.events` (app layer trusts the topic, doesn't re-verify per message).
- [contract-boundary QUESTION] follow-up: `team-order` CancelOrder ReleaseStock sets no reservation_id (pre-existing) — give cancel-driven releases the same idempotency as saga compensation.
- [note] W2 is CODE-COMPLETE + unit-green; end-to-end PAID/crash-release is proven only after W3 wiring + the purchase-saga e2e. Do not call W2 "done" end-to-end until then.

## PLAN COMPLETE (2026-09-03)
All waves done. purchase-saga + saga-compensation e2e PASS (event-carried PAID proven end-to-end). repo_doctor clean, validate_plan clean, FEATURES 50/50. Residual: OpenSearch e2e not run (host :9200 held by unrelated container — search fixes unit-verified only); follow-ups tracked below.
