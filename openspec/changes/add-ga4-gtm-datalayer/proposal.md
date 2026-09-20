# Proposal: Add GA4 / GTM Data Layer Pattern

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

## Non-goals

- Server-side GA4 Measurement Protocol / third-party ad network pixels (kept decoupled).
- Financial accounting from `tracking_events` (`order_facts` remains the single source of truth).
