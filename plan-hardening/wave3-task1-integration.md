# [W3-T1] Integration: netpol + wiring + e2e + gates

## Role
SRE  # + SA — wiring, deploy delta, convergence gate

## Objective
Everything is wired and green together: the new topics/tables/env exist, the
zero-trust NetworkPolicy lands (ADR-0010), the golden purchase-saga e2e passes,
and all gates are clean.

## Write-set (EXCLUSIVE) — integration wave owns the registration/wiring seams
Part A — wiring & fixes (verify per-repo in Docker):
- team-payment/cmd/server/main.go, team-payment/internal/bootstrap/**   (wire PgTxWriter+Relayer; drop dead UpdateOrderStatus)
- team-domain/cmd/server/main.go, team-domain/internal/bootstrap/**, team-domain/internal/handler/listing.go  (ReserveStockIdempotent; start sweeper)
- team-domain/internal/handler/listing_test.go              (fix stale API: Suggest/ListingStatus/PageSize)
- team-search/internal/handler/search_test.go               (fix stale API: Suggest/contextWithPrincipal)
- team-order/cmd/server/main.go, team-order/internal/bootstrap/**, team-order/go.mod  (wire Postgres saga repo + PaymentConsumer.Run + sweeper goroutine; add franz-go)
- team-order/internal/handler/order.go                      (gate ForceFailSaga + GetSagaState: RequirePrincipal + admin/owner)
Part B — infra & e2e (the heavy, user-checkpointed step):
- docker-compose.services.yaml                     (payment.events + *.dlq topics, saga/outbox env)
- platform-gitops/**                                (NetworkPolicy: only gateway reaches :5005x; Kafka ACL note)
- team-*/FEATURES.yaml, platform-core/docs/**       (mark hardened; cross-links — NOT ADR bodies)

## Architecture decision (SA, made here — not delegated)
- **team-order gains a Kafka consumer** → add `github.com/twmb/franz-go` (the house
  Kafka lib already used by team-domain/team-search). This is the platform-standard
  client, not a new platform dependency; consistent with AD6 "reuse existing patterns".

## Read-only dependencies
- All Wave 0-2 outputs (proto, per-repo code, migrations).

## Contracts
- H4: NetworkPolicy restricts service gRPC ports so only team-gateway (+ intra
  namespace as required) reaches them; documented as the interim zero-trust step
  (mTLS deferred per ADR-0010).
- Register: `payment.events` + `*.dlq` topics; payment outbox + team-order saga/
  reservations tables; any new env (TTL, retry max).

## Acceptance criteria
- [ ] `make -C platform-e2e up` + `pytest -k purchase_saga` green (golden path intact).
- [ ] Kill team-order mid-checkout in a test env → stock is released (no leak); order not left orphan PENDING.
- [ ] A forced OpenSearch error routes the event to `.dlq` and it is replayable.
- [ ] `python scripts/repo_doctor.py` clean; `python scripts/validate_plan.py plan-hardening/` clean.
- [ ] `docker compose -f docker-compose.services.yaml config` valid.

## Review
contract-boundary-reviewer + auth-scope-reviewer over the full diff. Rubric: SRE + SA.

## Verify
make -C platform-e2e up && pytest -k purchase_saga

## Out of scope
- No new features. No changes to the old plan/. Do not re-open ADR bodies (W0 owns them).
