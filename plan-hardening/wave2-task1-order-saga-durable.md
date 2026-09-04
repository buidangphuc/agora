# [W2-T1] team-order: durable saga + compensation + payment consumer

## Role
SE  # backend service — the heaviest seam; solo (single repo, coupled files)

## Objective
The purchase saga is durable and recoverable: reservation/saga state is persisted
before external effects, compensations run on a fresh context and are retried/parked
(never discarded), multi-seller failure compensates only un-committed work, the
order transitions to PAID by consuming `PaymentSettled` idempotently, and
returns/shipments survive restart.

## Write-set (EXCLUSIVE)
- team-order/internal/service/order.go             (edit)
- team-order/internal/service/saga*.go             (create — saga state + compensation)
- team-order/internal/repository/**                (edit/create — saga+returns+shipments Postgres)
- team-order/internal/consumer/**                  (create — payment.events consumer)
- team-order/migrations/**                          (create — saga_state, reservations, returns, shipments)
- team-order/internal/**/*_test.go                 (create/edit)

## Read-only dependencies
- W0: `platform.payment.v1.PaymentSettled` (run `buf generate` here — codegen).
- W1-T2 contract: `ReserveStockRequest.reservation_id` is honored idempotently by team-domain.
- W1-T3: PaymentSettled is emitted on `payment.events`.

## Contracts
- AD3: persist saga/reservation state BEFORE external calls. Compensations use
  `context.Background()` + own deadline; failed `ReleaseStock` retried/parked,
  errors never `_, _ =` discarded.
- AD5: caller SETS a stable `reservation_id` per (cart_item, attempt).
- AD4: consume `PaymentSettled` (key=order_id) idempotently → transition order to
  PAID exactly once (dedupe on event_id / payment_id).
- M7: multi-seller — persist all seller-orders in one tx OR scope compensation to
  un-committed orders; never release stock of already-persisted orders.
- M8: returns & shipments use Postgres repos (not in-memory) outside tests.
- Cart `RemoveItems` error after order creation must be handled, not ignored.

## Acceptance criteria
- [ ] Test: crash/timeout after ReserveStock but before persist → saga recovery/TTL releases stock; no orphan PENDING order. (reproduces C2)
- [ ] Test: compensation runs even when the request ctx is already cancelled (bg ctx). 
- [ ] Test: same reservation_id retried → single stock decrement (with W1-T2).
- [ ] Test: consuming PaymentSettled twice transitions the order once (idempotent). (reproduces H3 consumer)
- [ ] Test: multi-seller partial failure does not orphan a persisted order's stock. (M7)
- [ ] Test: returns/shipments persist across a repo restart (Postgres). (M8)
- [ ] `go test ./...` green in Docker.

## Review
BOTH contract-boundary-reviewer (saga, broker consumer, cross-service calls) AND
auth-scope-reviewer (order RPC authorization unchanged). Rubric: SE section.

## Verify
docker compose -f docker-compose.services.yaml run --rm team-order go test ./...

## Out of scope
- No frontend, no proto edits (consume the W0 contract). No PM features.
- Do not modify team-domain reserve internals (only call it with reservation_id).
