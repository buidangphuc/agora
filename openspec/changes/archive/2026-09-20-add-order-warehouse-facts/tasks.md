# Tasks: Order Events Outbox & Warehouse `order_facts`

## Track 1: Contract & Proto (platform-core)
- [x] 1.1 Add `OrderPaidEvent` and `OrderLineItemFact` to `platform-core/packages/proto/platform/order/v1/order.proto` <!-- id: proto-order-paid -->
- [x] 1.2 Vendor proto and regenerate stubs in `team-order` and `team-analytics` <!-- id: vendor-proto -->

## Track 2: team-order Outbox & Event Emission
- [x] 2.1 Add migration `0006_order_outbox.up.sql` and down sql for `order_outbox_events` <!-- id: order-outbox-migration -->
- [x] 2.2 Implement `OutboxRepository` in `team-order/internal/repository/outbox.go` and `outbox_pg.go` <!-- id: order-outbox-repo -->
- [x] 2.3 Implement background relayer in `team-order/internal/events/relayer.go` and publisher in `publisher.go` <!-- id: order-relayer -->
- [x] 2.4 Wire outbox event insertion into order state transition to `PAID` in `team-order/internal/service/` <!-- id: order-emit-paid -->

## Track 3: team-analytics Warehouse & Query Rewriting
- [x] 3.1 Define `OrderFactRecord` and `OrderFactsSchema` in `team-analytics/internal/warehouse/warehouse.go` <!-- id: analytics-order-schema -->
- [x] 3.2 Update DuckDB adapter in `team-analytics/internal/warehouse/duckdb/duckdb.go` for `order_facts` table <!-- id: analytics-duckdb-order-facts -->
- [x] 3.3 Update BigQuery adapter in `team-analytics/internal/warehouse/bigquery/bigquery.go` for `order_facts` table <!-- id: analytics-bigquery-order-facts -->
- [x] 3.4 Implement consumer for `order.events` in `team-analytics/internal/consumer/order.go` <!-- id: analytics-order-consumer -->
- [x] 3.5 Rewrite `GetRevenueBreakdown` and `GetSellerFunnel` in `team-analytics/internal/query/duckdb.go` to read `order_facts` <!-- id: analytics-query-rewrite -->

## Track 4: E2E & Validation
- [x] 4.1 Update `team-analytics/FEATURES.yaml` and `team-order/FEATURES.yaml` <!-- id: update-features-yaml -->
- [x] 4.2 Create `platform-e2e/tests/e2e/features/analytics/order_facts.feature` <!-- id: e2e-order-facts-feature -->
- [x] 4.3 Implement step definitions in `platform-e2e/tests/e2e/step_definitions/test_order_facts.py` <!-- id: e2e-order-facts-steps -->
- [x] 4.4 Verify unit tests, `openspec validate add-order-warehouse-facts --strict`, and E2E coverage <!-- id: verify-all -->

