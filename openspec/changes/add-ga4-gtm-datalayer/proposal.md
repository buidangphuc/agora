# Proposal: Add GA4 / GTM Data Layer Pattern

initiative: adhoc (prerequisite of initiative `ga4-analytics`)

## Why

Currently, behavioral telemetry in Agora is coupled directly from UI components to `/api/track` without a decoupled event bus (`window.dataLayer`), lacking standard ecommerce metadata (`currency`, `value`, `price`, `quantity`, `transaction_id`, `coupon`, `item_category`, `item_list_id`). Furthermore, multi-item grids issue unbatched requests, seller funnels lack checkout/purchase stages, and warehouse tables lack auto-migration for evolved tracking attributes.

This change introduces the industry-standard **GA4 / GTM Data Layer Pattern**:
1. Decoupled client-side `dataLayer` event bus with automatic `{ ecommerce: null }` reset.
2. High-performance batching queue with automatic flush triggers.
3. Expanded Protobuf schema supporting rich ecommerce interaction events (`view_cart`, `add_shipping_info`, `add_payment_info`, `purchase`) and item fan-out attribution.
4. Edge collector supporting GA4 event aliases and multi-item correlation.
5. Idempotent warehouse table evolution and `ga4_events` read view.

## What Changes

- **Contract** (`platform-core/packages/proto`):
  - Add `EventType` enums: `VIEW_CART = 11`, `ADD_SHIPPING_INFO = 12`, `ADD_PAYMENT_INFO = 13`, `PURCHASE = 14`.
  - Add fields 13..24 to `TrackingEvent`: `currency`, `value`, `price`, `quantity`, `transaction_id`, `coupon`, `item_category`, `item_list_id`, `item_list_name`, `event_group_id`, `shipping_tier`, `payment_type`.
  - Add `begin_checkouts = 5` and `purchases = 6` to `GetSellerFunnelResponse`.
- **Frontend** (`team-frontend`):
  - Create `src/lib/analytics/` module (`dataLayer`, batch `queue`, `map`, `dispatcher`).
  - Add `<AnalyticsProvider>` in `src/app/layout.tsx`.
  - Convert `src/lib/track.ts` to backward-compatible shim.
  - Migrate call sites to batched ecommerce dispatch.
- **Edge Gateway** (`team-gateway`):
  - Expand `collector.go` to parse ecommerce fields, accept GA4 event aliases, and enforce batch size caps.
- **Warehouse** (`team-analytics`):
  - Append columns to `TrackingRecord` and `Schema`.
  - Add idempotent `ALTER TABLE ADD COLUMN IF NOT EXISTS` migration in DuckDB and BigQuery adapters.
  - Expose `ga4_events` view and expand `GetSellerFunnel` SQL.
- **RecSys** (`platform-recsys`):
  - Add event interaction weights and expand `TRACKING_COLUMNS`.
- **E2E** (`platform-e2e`):
  - Add BDD scenarios asserting `dataLayer` structure, single-request grid batching, and fan-out correlation.

## Locked assumptions

- p99 < 50 ms is measured on the handler only (fake publisher, N=1000, 100-event batch); broker latency is out of scope.
- GA4 aliases in scope are exactly those in `beaconEventTypes` in `team-gateway/internal/edge/collector.go`: `view_item` -> view, `select_item` -> click, `view_item_list` -> impression. Other GA4 names (for example `add_to_wishlist`, `select_promotion`) are not accepted and are dropped as unknown event types.
- An "invalid" beacon means an unknown or empty event type (or a field breaking a length/count bound); it is dropped while its siblings are kept.
- The funnel `purchases` count is DISTINCT `transaction_id` so replayed beacons do not inflate it; purchase rows with an empty `transaction_id` are each counted.
- Retention: raw `tracking_events` rows are kept 13 months, after which only aggregates are kept (human decision). The new columns follow the same policy. Enforcement (TTL job / BigQuery partition expiration) is a follow-up outside this change; team-analytics has no existing prune mechanism. (Database backup `RETENTION_DAYS=7` in `platform-gitops` is not row retention.)
- Vendored protos (`team-gateway/proto`, `team-analytics/proto`, `team-frontend/proto`) already carry fields 13-24, enums 11-14 and `begin_checkouts`/`purchases`; `buf lint` and `buf breaking` (against main) pass in `platform-core/packages/proto`.

## Non-goals

- Server-side GA4 Measurement Protocol / third-party ad network pixels (kept decoupled).
- Financial accounting from `tracking_events` (`order_facts` remains the single source of truth).

## Non-functional requirements

- Telemetry dispatch is non-blocking and never throws into UI code; client queue flushes at 20 events, 2 s idle, or `visibilitychange: hidden`.
- Edge `/api/track` accepts at most 100 events per request and at most 64 KiB body; p99 handler latency stays under 50 ms for a 100-event batch (verified by a Go benchmark of the handler with a fake publisher, N=1000 requests of 100 events each; the broker path is excluded).
- Warehouse migrations are idempotent: running startup N times yields the same schema, with no downtime and no data rewrite.
- Parquet export is backward compatible: readers handle files with and without the new columns.
- With `NEXT_PUBLIC_GTM_ID` empty (dev, CI, e2e) no third-party script or request is made.

## Data classification

| Field | Class | Purpose | Consent basis | Retention |
|---|---|---|---|---|
| `item_list_id`, `item_list_name`, `position`, `item_category` | operational, non-personal | list attribution, ranker features | legitimate interest (necessary operational stats) | 13 months raw, then aggregates only (see Locked assumptions) |
| `currency`, `value`, `price`, `quantity`, `coupon` | operational, non-personal | funnel analytics; never accounting | legitimate interest | 13 months raw, then aggregates only |
| `transaction_id` | business identifier (order id, pseudonymous) | dedupe `purchase` events | legitimate interest | 13 months raw, then aggregates only |
| `event_group_id` | random UUID, non-personal | correlate fan-out rows | legitimate interest | 13 months raw, then aggregates only |
| `shipping_tier`, `payment_type` | operational enums | checkout funnel | legitimate interest | 13 months raw, then aggregates only |

No email, phone, name, address or card data appears in any dataLayer push, beacon, proto field, warehouse column or Parquet file.

## Threat model

- Assets: warehouse integrity, funnel and recsys training data quality, revenue figures.
- Abuse cases: (1) oversized or huge batches to exhaust the edge (mitigated by 100-event and 64 KiB caps, atomic reject); (2) forged `purchase` or inflated `value` to skew revenue (mitigated: `order_facts` is the sole revenue source, tracking values are never summed into revenue); (3) replayed `purchase` beacons double-counting conversions (mitigated by dedupe on `transaction_id`); (4) PII smuggled in `properties` or new string fields (mitigated by existing length caps and scrubbing; no PII fields in schema); (5) schema migration run by a non-owner (mitigated: runs only in the worker with warehouse credentials).
- Authz rule: `/api/track` is public (anonymous allowed, principal stamped by edge when present); warehouse and Parquet access stays restricted to service identities; no new user-facing read endpoint is added.
