# Tasks: payment-settlement-commission

Scenario ids are the scenario titles in `specs/seller-commission/spec.md`, `specs/seller-settlement-credit/spec.md`
and `specs/seller-payout-holdback/spec.md`. Depends on `order-paid-price-breakdown` (event fields) and, for the
shared ledger CHECK, is serialised with `payment-seller-fee-charge`.

## Code track (team-payment, team-gateway)
- [ ] 1. Sync consumer copies of `payment.proto` and `order.proto` (new `OrderPaidEvent` fields 9-12, wallet types,
  `GetOrderSettlement`) into `team-payment/proto` and `team-gateway/proto`, regenerate. Scenarios: "A paid order
  books 5% on subtotal minus shop voucher", "Another seller gets NOT_FOUND".
- [ ] 2. Config: `Commission` section with `COMMISSION_RATE_BPS` default 500, valid 0..3000, refuse start naming the
  key; README and `.env.example`. Scenarios: "An invalid rate refuses to start", "The boundary rates are accepted".
- [ ] 3. Migration `0018` (up and down): extend `wallet_ledger_type_sign_chk` with `COMMISSION` < 0 and
  `COMMISSION_REVERSAL` > 0, reference-required CHECK for both, add nullable `commission_rate_bps`; keep the
  `SUBSCRIPTION_FEE` arm from `payment-seller-fee-charge` if already merged. Scenarios: "The store refuses a second
  commission for one payment", "The store refuses a commission that is not negative", "The store refuses a reversal
  that is not positive", "The store refuses a commission without a reference", "The migration keeps existing ledger
  rows valid".
- [ ] 4. `store/ledger.go`: ledger type constants, `Commission(...)` and `CommissionReversal(...)` entry builders,
  pure `CommissionAmount(base, bps)` and `ReversalTarget(commission, refunded, paid)` helpers (integer, no overflow).
  Scenarios: "Commission rounds half up to whole VND", "Cumulative rounding never exceeds the commission", "A zero
  commission writes no row".
- [ ] 5. `store/settlement.go` `CreditSettlement`: accept the breakdown, credit paid + platform discount, insert the
  `COMMISSION` row in the same transaction, then per-refund deductions and reversals with a running cumulative sum;
  `ApplyRefund`: reversal after the deduction in the same transaction. Scenarios: "A paid order books 5% on subtotal
  minus shop voucher", "The platform voucher and shipping are outside the base", "A redelivered paid event books no
  second commission", "Concurrent settlements of one order book one commission", "A failed commission insert rolls
  back the credit", "A 40% partial refund returns 40% of the commission", "A full refund returns exactly the
  commission", "A tiny refund whose share rounds to zero books no reversal", "Cancelling a paid order returns the
  whole commission", "A refund that landed before the credit still gets its reversal", "A replayed refund writes no
  second reversal", "A payment without commission is refunded without a reversal", "Concurrent refunds never reverse
  more than the commission".
- [ ] 6. `settlement/handler.go` `handlePaid`: read the four optional fields, treat absent as no breakdown (metric
  `payment_commission_skipped_total{reason="no_breakdown"}`), inconsistent as a permanent error to the DLQ; thread
  through `payment.Service.CreditSettlement`. Scenarios: "An event without the breakdown books no commission", "An
  event without the price breakdown credits the paid amount only", "An inconsistent breakdown is parked and writes
  nothing", "A platform voucher is credited to the seller on top of the paid amount".
- [ ] 7. `heldCreditsSQL` nets `COMMISSION` and `COMMISSION_REVERSAL`. Scenarios: "A held sale is held net of its
  commission" plus the existing holdback scenarios unchanged.
- [ ] 8. `GetOrderSettlement` in `internal/rpc` and `Service`: authz on the credit's seller or admin, NOT_FOUND
  otherwise, INVALID_ARGUMENT on empty id, read-only. Scenarios: "The seller reads the breakdown of a settled order",
  "The breakdown reflects a partial refund", "An order settled without commission reports zero commission", "Another
  seller gets NOT_FOUND", "A buyer gets NOT_FOUND", "An admin can read any settlement", "An unsettled order is not
  found", "A missing order id is rejected", "Reading the settlement writes nothing".
- [ ] 9. `team-gateway/internal/edge/payment.go`: `GetOrderSettlement` forwarder with `callRead`; confirm
  `ChargeSellerFee` is not forwarded. Scenarios: "An unauthenticated call is refused", "The seller reads the
  breakdown of a settled order", "A client cannot set or forge a commission".
- [ ] 10. Property and benchmark tests: reconcile after shuffled duplicated events; p95 settlement overhead at most
  5 ms. Scenarios: "The ledger reconciles after any order of events", "Commission adds at most 5 ms p95 to
  settlement". Run Docker-backed tests with `GO_SDK_REQUIRE_DOCKER=1`.

## E2E track (platform-e2e)
- [ ] 11. Flow, actor buyer + seller: place and pay an order with subtotal 300000, shipping 30000, shop voucher
  20000; the seller's ledger shows `ORDER_SETTLEMENT` +310000 and `COMMISSION` -14000 and `GetOrderSettlement` net
  296000. Scenarios: "A paid order books 5% on subtotal minus shop voucher", "The seller reads the breakdown of a
  settled order".
- [ ] 12. Flow: order with a platform voucher; seller credit is paid + platform voucher, commission excludes it.
  Scenarios: "A platform voucher is credited to the seller on top of the paid amount", "The platform voucher and
  shipping are outside the base".
- [ ] 13. Flow: partial return refund of 40% then the rest; reversals 5600 then 8400. Scenarios: "A 40% partial
  refund returns 40% of the commission", "A full refund returns exactly the commission".
- [ ] 14. Flow: cancel a paid order; one reversal of the full commission. Scenario: "Cancelling a paid order returns
  the whole commission".
- [ ] 15. Flow: replay the `OrderPaidEvent` record; one commission. Scenario: "A redelivered paid event books no
  second commission".
- [ ] 16. Flow, permission: another seller, a buyer and an anonymous caller call `GetOrderSettlement`; admin succeeds.
  Scenarios: "Another seller gets NOT_FOUND", "A buyer gets NOT_FOUND", "An admin can read any settlement", "An
  unauthenticated call is refused", "A refund by a seller who is not the order's seller is denied and reverses nothing".
- [ ] 17. Flow: fresh settlement is held net of commission; payout over the net is refused. Scenario: "A held sale is
  held net of its commission".
