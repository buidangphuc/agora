## Why

`platform-featurestore` dropped the user feature `paid_orders_30d` from `user_activity@v1` because `order_facts` has no
buyer column (`featurestore-materialization`, follow-up recorded in its tasks). The event that feeds the table,
`OrderPaidEvent`, already carries `buyer_id` and team-order already fills it. Only team-analytics throws it away.
`recs-attribution-hardening` also needs the buyer to count purchases server-side.

## What Changes

- **team-analytics:** `order_facts` gets a nullable `buyer_id` column (DuckDB and BigQuery schemas stay in parity). The
  order consumer stores `OrderPaidEvent.buyer_id` on every line row. Existing databases are migrated in place; their old
  rows keep a NULL buyer. The `order_facts.parquet` export carries the column.
- **platform-featurestore:** `user_activity` moves to **v2**: v1's five features plus `paid_orders_30d`, the number of
  distinct paid orders of the user in the 30 days before `AS_OF`. `user_activity@v1` is retired from the registry
  (nothing reads it). The `orders` input requires the `buyer_id` column. README and lock updated.
- **platform-e2e:** scenarios for every spec scenario (`ofb_` step module, own feature file); FEATURES.yaml entries.

Repos touched: team-analytics, platform-featurestore, platform-e2e (new files only). **No proto change**:
`platform.order.v1.OrderPaidEvent.buyer_id` (field 2) exists and `team-order`'s `BuildOrderPaidEnvelope` fills it.

## Capabilities

### New Capabilities
- None.

### Modified Capabilities
- `order-facts`: order facts record the buyer, and the export carries it.
- `feature-materialization`: `user_activity@v2` with `paid_orders_30d`; v1 of that view is retired.

## Non-goals

- Back-filling the buyer on rows ingested before the change (the Kafka topic is the only source; replaying it is an ops
  decision). Those rows stay unattributed.
- Refunds, cancellations or returns lowering `paid_orders_30d` (order_facts is append-only PAID facts).
- Serving the feature to team-ai or any ranking change.
- Consent, retention or erasure of the new personal-data column (legal is deferred).
