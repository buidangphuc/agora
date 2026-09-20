## Why

`AnalyticsQueryService.GetRevenueBreakdown` and `GetSellerFunnel` attempt to calculate seller revenue and conversion by projecting fields out of an unstructured JSON bag in `tracking_events` (`json_extract_string(properties, '$.revenue')`, `TRY_CAST`). However, `analytics.events` is only produced by client-side browser beacons (`TrackEventType: view | click | add_to_cart | impression`), which contain no purchase events, are best-effort, and can be spoofed or fail silently. Furthermore, `team-order` currently emits no Kafka events upon order settlement.

Per **ADR-0013**, authoritative purchase facts must enter the warehouse from the backend (`team-order`) on the `PAID` state transition, written through a transactional outbox to `order.events`. `team-analytics` must consume these events into a dedicated `order_facts` table (line-item grain) and rewrite seller queries to aggregate directly from `order_facts`.

## What Changes

- **platform-core** (`packages/proto/platform/order/v1/order.proto`):
  - Add `OrderPaidEvent` and `OrderLineItemFact` event definitions for Kafka payload.
- **team-order**:
  - Add migration `0006_order_outbox.up.sql` defining `order_outbox_events`.
  - Implement transactional outbox repository (`internal/repository/outbox.go`, `outbox_pg.go`).
  - Implement background outbox relayer (`internal/events/relayer.go`) publishing to Kafka `order.events`.
  - Emit `OrderPaidEvent` atomically when an order transitions to `PAID`.
- **team-analytics**:
  - Add `OrderFactRecord` and `OrderFactsSchema` in `internal/warehouse/warehouse.go`.
  - Implement `order_facts` table management in DuckDB and BigQuery adapters.
  - Implement Kafka consumer for `order.events` in `internal/consumer/order.go`.
  - Rewrite `RevenueBreakdown` and `SellerFunnel` in `internal/query/duckdb.go` and `repository.go` to aggregate from `order_facts`.
- **platform-e2e**:
  - Add `order_facts.feature` and step definitions verifying outbox emission, warehouse ingestion, and rewritten query accuracy.

## Non-goals

- No ML forecasting model in this change (forecasting baseline in `platform-forecast` is Stage 1).
- No modifications to client-side browser tracking beacon (`team-frontend/src/lib/track.ts`).
- No mutation of historic records upon refund/cancellation (corrections are append-only compensating events).
