# [W1-T2] team-order — voucher redemption in the checkout saga

## Role
SE   # saga durability; idempotent external effect

## Objective
CreateOrder accepts `voucher_code`, calls promotion `ValidateAndReserve` during the saga, records `discount_amount` on the order, `CommitReservation` on PaymentSettled, `ReleaseReservation` on saga failure/cancel. total_amount reflects the discount.

## Write-set (EXCLUSIVE)
- team-order/internal/service/redemption.go        (create — reserve/commit/release helpers + hooks into saga)
- team-order/internal/upstream/promotion.go        (create — promotion gRPC client wrapper)
- team-order/internal/service/saga.go              (edit — call redemption at reserve step + compensation)
- team-order/internal/service/order.go             (edit — pass voucher_code, set discount_amount/total)
- team-order/internal/consumer/payment.go          (edit — Commit on PaymentSettled)
- team-order/internal/service/*_test.go            (edit/create — redemption + saga tests)
- team-order/migrations/0005_voucher_redemption.up.sql / .down.sql (create — add voucher_code, discount_amount to orders)

## Read-only dependencies
- team-order generated stubs (promotion client from Wave 0), existing saga/order/cart code
- 00-spec.md §Contracts F1 (promotion RPCs + order additive fields)

## Contracts
Order saga: persist saga state → `ValidateAndReserve(reservation_id=<saga_id>, code, buyer, cart_subtotal, seller)` → on success store discount; `total_amount = items_subtotal + shipping_fee - discount_amount` (floor 0). On `PaymentSettled` consume → `CommitReservation(saga_id)`. On saga fail/cancel → `ReleaseReservation(saga_id)`. Empty voucher_code = skip promotion entirely (unchanged path).

## Acceptance criteria
- [ ] Order with valid voucher has discount_amount>0 and reduced total; without voucher unchanged.
- [ ] Invalid/expired voucher → order still succeeds at full price OR fails per policy (test asserts chosen behavior; default: reject with FailedPrecondition, do not silently drop).
- [ ] Reservation released on saga compensation (test: force-fail saga → promotion Release called).
- [ ] Idempotent: reusing saga_id as reservation_id does not double-apply.
- [ ] gofmt/go vet/go test clean; promotion client mocked in tests.

## Review (gate — different agent)
Route to **contract-boundary-reviewer** (saga + cross-service call). Rubric = SE.

## Verify
```bash
# in Docker: (cd team-order && gofmt -l . && go vet ./... && go test ./...)
```

## Out of scope
- Do NOT implement promotion service logic (W1-T1).
- Do NOT edit proto/generated. No gateway/frontend. No changes to payment service.
