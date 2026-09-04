# [W1-T1] team-promotion — voucher + flash-sale business logic

## Role
SE   # 3-test minimum; handler/service/repository trio; idempotent reserve

## Objective
team-promotion serves VoucherService + FlashSaleService for real: CRUD, idempotent ValidateAndReserve→Commit/Release, active flash-sale lookup + live stock, and emits `promotion.events`.

## Write-set (EXCLUSIVE)
- team-promotion/internal/handler/**        (create — voucher.go, flashsale.go + _test.go)
- team-promotion/internal/service/**        (create — voucher.go, flashsale.go + _test.go)
- team-promotion/internal/repository/**     (create — voucher.go, flashsale.go, reservation.go, Postgres + in-memory + _test.go)
- team-promotion/internal/producer/**       (create — promotion.events emitter)
- team-promotion/migrations/0001_promotion.up.sql / .down.sql (edit — fill tables if skeleton left them empty)

## Read-only dependencies
- team-promotion generated stubs + skeleton (Wave 0)
- team-order/internal/{handler,service,repository}/cart.go (CRUD pattern), internal/repository/saga.go (reservation idempotency pattern)
- 00-spec.md §Contracts F1

## Contracts
promotion.proto (00-spec.md). Key invariant: `ValidateAndReserve` idempotent on `reservation_id` — same id returns the same discount without double-decrementing quota. `Commit` finalizes (increment `used`); `Release` frees the hold. Discount math: PERCENT → `min(subtotal*value/100, max_discount)`; FIXED → `min(value, subtotal)`; reject if `subtotal < min_spend`, expired, or quota exhausted.

## Acceptance criteria
- [ ] ValidateAndReserve idempotent on reservation_id (test: call twice → one hold, same discount).
- [ ] Reject expired / below-min-spend / quota-exhausted with a reason string, never a panic.
- [ ] Flash-sale GetFlashSaleStock returns remaining = cap - sold; active window respected.
- [ ] promotion.events emitted on voucher/campaign create (EventEnvelope with principal+traceparent).
- [ ] Repo tests cover Postgres + in-memory; ≥3 service tests; gofmt/go vet clean.

## Review (gate — different agent)
Route to **contract-boundary-reviewer** (emits Kafka, checkout seam). Rubric = SE section of experts.md.

## Verify
```bash
# in Docker: (cd team-promotion && gofmt -l . && go vet ./... && go test ./...)
```

## Out of scope
- Do NOT edit proto, main.go, grpcserver, config (Wave 0 owns them).
- Do NOT edit team-order (that's W1-T2).
- No real payment/wallet calls (§7). No gateway/frontend.
