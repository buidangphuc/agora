# Proposal: order-cod-sales-count

initiative: discovery-followups

## Why
`sold_count` ("Bán chạy") is counted only when an order moves to PAID. COD orders go
PENDING -> SHIPPED -> COMPLETED and never reach PAID, so they never count. COD sellers are under-ranked in
discovery. `discovery-seo` left this as an open issue.

## What changes
- team-order counts a COD order's lines into `listing_sales` when it moves SHIPPED -> COMPLETED, inside the
  same compare-and-set transaction in `UpdateOrderStatusFrom` (`countOrderSales`), and writes the existing
  `ListingSalesChanged` outbox row. Buyer, seller and the auto-complete sweep all reach this one place.
- An online-paid order that completes is not counted again: it was counted at PAID, and the per-order ledger
  `listing_sales_orders` is the single idempotency guard.
- A new idempotent migration (0024, event ids include the migration id) backfills historical COD orders already COMPLETED: ledger rows,
  `sold_count` increment, and one `ListingSalesChanged` row per affected listing.
- Not changed: no `OrderPaidEvent` for COD (team-payment would credit the seller wallet), no proto change, no
  new event, no reversal of `sold_count` for COD orders. Reason: a counted COD order is COMPLETED, which is
  terminal (it can never be cancelled), and a COD return can never reach REFUNDED (approval is refused: "order
  was not paid online; cash-on-delivery refunds are handled outside the system"), so no reversal trigger exists. If COD refunds are ever enabled, a COD reversal path is needed.

## NFR
- Counting adds no new transaction or RPC; it runs in the existing status CAS. Added cost: one ledger insert
  and one upsert per distinct listing in the order.
- Latency: SHIPPED -> COMPLETED p95 rises by at most 5 ms over the pre-change baseline, measured locally,
  for orders up to 10 lines (scenario "Completing a COD order stays within the latency budget").
- Event reaches `order.events` within 5 s of commit (existing relay SLO).
- Migration is re-runnable with no change in totals or outbox row count.

## Data classification
| Field | Purpose | Consent basis | Retention |
|---|---|---|---|
| `listing_sales.listing_id`, `sold_count` | ranking signal for discovery ("Bán chạy") | none required, no personal data | kept while the listing exists |
| `listing_sales_orders.order_id`, `listing_id`, `quantity`, `state` | idempotency ledger so an order is counted once | none required, no personal data (order id is a business key, no party identifier) | follows order retention |
| `ListingSalesChanged{listing_id, sold_count, changed_at}` | publish the absolute count to search | none required, no personal data | outbox retention, unchanged |

## Threat model
- Assets: integrity of `sold_count` (ranking signal).
- Abuse: a seller or buyer completing an order repeatedly or concurrently to inflate the count; a client
  sending `sold_count`; completing a COD order that was never shipped.
- Authz rule: unchanged. Who may complete an order is governed by existing order RPC rules; counting is a
  side effect of a successful CAS only. The CAS row lock plus ledger primary key (order_id, listing_id)
  ensure at most one count per order and listing. No RPC accepts `sold_count`.
