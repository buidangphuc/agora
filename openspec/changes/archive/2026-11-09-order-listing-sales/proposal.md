## Why

Search cannot sort by "Bán chạy" because no service owns a per-listing sold count. initiative `discovery-seo` (stories 1-2, decisions D1 and D2) makes team-order the owner: it already knows when an order is paid, cancelled from PAID, or refunded through a return, so it keeps a `listing_sales` projection and publishes the absolute value for the team-search indexer to project.

initiative: discovery-seo

## What changes

- Migration `0023_listing_sales` in `team-order/migrations` adds two tables.
  - `listing_sales (listing_id PK, sold_count BIGINT NOT NULL CHECK (sold_count >= 0), updated_at)`.
  - `listing_sales_orders (order_id, listing_id, quantity, state, PRIMARY KEY (order_id, listing_id))`, with `state` either `counted` or `reversed`. This is the per-order ledger that makes the projection idempotent.
- The same migration backfills both tables from existing rows and enqueues one `ListingSalesChanged` outbox row per listing that has at least one order line (see design.md).
- The projection is updated inside the transaction that already writes `OrderPaidEvent`, `OrderCancelled` (only when `previous_status` is PAID) and `ReturnRefunded` to `order_outbox_events`. When the value changes, a `ListingSalesChanged{listing_id, sold_count, changed_at}` outbox row is written in that same transaction, produced by the existing relayer to `order.events`, keyed by `listing_id`.
- Decision (C2): a refunded return reverses the whole order's quantity. Returns are whole-order today, so partial refunds are out of scope until returns carry line items.
- Deploy order (C9): migration `0023` ships in the same deploy as the new writer binary, so no transition happens between the schema and the writer.
- No new RPC and no new topic. The proto message `ListingSalesChanged` comes from `discovery-seo-contract`.

## NFR

- Event reaches `order.events` within 5 s of the source transition on the local stack (relayer cadence unchanged), leaving search 25 s of the 30 s end-to-end budget.
- The projection update adds at most one extra statement pair per distinct listing in the order and no extra round-trip outside the existing transaction. p95 of the Paid, CancelOrder and refund paths must not regress more than 10% against the local baseline.
- Benchmarks (measured, non-gating): 100,000-line backfill time and p95 of the Paid, CancelOrder and refund paths, recorded in the task 1.7 output.
- Backfill: finishes in under 60 s for 100,000 order lines and enqueues exactly one row per listing; the migration is re-runnable without duplicating the ledger or the seed rows.
- Delivery is at-least-once; consumers are safe because the payload is absolute.

## Data classification

| field | class | purpose | consent basis | retention |
|---|---|---|---|---|
| `listing_sales.listing_id`, `sold_count` | non-personal business data | best-selling sort | not personal data, none needed | life of the listing |
| `listing_sales_orders.order_id`, `listing_id`, `quantity`, `state` | pseudonymous business key (order id links to a buyer only through the `orders` table) | idempotency ledger | contract performance, same as `orders` | follows `orders` rows (deleted with them) |
| `ListingSalesChanged` payload | non-personal: no buyer, seller or order id | search projection | none needed | outbox retention unchanged |

No new personal data is collected, copied or logged.

## Threat model

- Assets: integrity of `sold_count`, which drives ranking and buyer trust.
- Abuse cases: a seller or buyer inflates sales by replaying events or by placing and cancelling orders (mitigated by the per-order ledger, by counting only PAID and by reversing on cancel-from-PAID or refund); a negative count from double reversal (mitigated by the ledger state and a `CHECK (sold_count >= 0)`); event forgery on the topic (unchanged, produced only by the team-order relayer).
- Authz rule: there is no new RPC and no user-facing write path. Only the existing order transitions, which already enforce ownership, can change the projection, and the value is never accepted from a request.
