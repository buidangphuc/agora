# Tasks: order-cod-sales-count

## Code track (team-order)
- [ ] 1. In `UpdateOrderStatusFrom` (internal/store/order.go), on `to == Completed` and
  `o.PaymentMethod == CodPaymentMethod`, call `countOrderSales` inside the same tx, guarded by
  `listingSalesOutbox != nil`; rely on the ledger primary key so a non-COD or already counted order adds
  nothing. Scenarios: "A COD order completed by the buyer is counted", "A COD order completed by the seller is
  counted", "A COD order completed by the auto-complete sweep is counted", "Completing an online-paid order
  does not count it again", "A repeated or concurrent COD completion counts once", "A COD completion that
  fails midway leaves no sales rows", "A COD order with several listings emits one event per listing".
- [ ] 2. Confirm no count on COD PENDING -> SHIPPED, on a cancel from PENDING, or on a PENDING -> COMPLETED attempt.
  No reversal code is added: COMPLETED is terminal and a COD return never reaches REFUNDED. Scenarios: "A COD
  order is not counted before it completes", "A COD order cannot jump from PENDING to COMPLETED", "A COD
  return request does not change the sold count".
- [ ] 3. Add migration `0024_listing_sales_cod` (up and down, embedded): insert `counted` ledger rows for COD
  COMPLETED orders with `ON CONFLICT DO NOTHING`, add only newly inserted quantity to `listing_sales`, enqueue
  one `ListingSalesChanged` per changed listing with event id `hash(listing, sold_count, '0024-backfill-cod')`
  (migration id included so it cannot collide with 0023 ids) and `ON CONFLICT (event_id) DO NOTHING`.
  Scenarios: "Historical COD completed orders are counted once", "The COD backfill ignores orders already in
  the ledger and non-COD orders", "Running the COD backfill twice does not duplicate", "The COD backfill on a
  database with no COD orders succeeds".
- [ ] 4. Unit and migration tests under `internal/store` and `migrations` for every scenario above, red first.
- [ ] 5. Latency benchmark: measure SHIPPED -> COMPLETED p95 before the change (baseline) and after, locally,
  orders of up to 10 lines, at least 200 completions, same machine and same run, median p95 of 3 runs; fail if p95 rises by more than 5 ms. Scenario:
  "Completing a COD order stays within the latency budget".
- [ ] 6. Verify no RPC or proto field accepts `sold_count`. Scenario: "A client cannot set the sold count" (add `TestNoRPCAcceptsSoldCount`, a proto field check).

## E2E track (platform-e2e)
- [ ] 7. Flow, actor buyer: place a COD order for L, ship, buyer completes; assert team-search `sold_count`
  for L rises by the quantity. Scenario: "A COD order completed by the buyer is counted".
- [ ] 8. Flow, actor seller: same, seller completes. Scenario: "A COD order completed by the seller is counted".
- [ ] 9. Flow, actor sweep: SHIPPED COD order past `auto_complete_at`, sweep runs, count rises. Scenario: "A COD
  order completed by the auto-complete sweep is counted".
- [ ] 10. Flow: complete an online-paid order; `sold_count` rises once at PAID and not at COMPLETED. Scenario:
  "Completing an online-paid order does not count it again".
- [ ] 11. Flow: complete the same COD order twice (second call rejected); one increase. Scenario: "A repeated
  or concurrent COD completion counts once".
