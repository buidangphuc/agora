## Context

See proposal.md. `OrderPaidEvent{order_id, buyer_id, items[], ...}` is consumed by
`team-analytics/internal/consumer/order.go`, which writes one `order_facts` row per line item
(`event_id = <envelope id>-<index>`). `OrderFactsSchema` (warehouse.go) is the single schema for DuckDB and BigQuery;
DuckDB adds missing columns on open (`ALTER TABLE ... ADD COLUMN IF NOT EXISTS`); BigQuery has `evolveSchema` for
the tracking table only. The featurestore job reads `order_facts.parquet` as the `orders` view.

## Goals / Non-Goals

**Goals:** buyer on every new fact row; export and feature built on it; no data loss for old rows.
**Non-Goals:** see proposal.

## Decisions

### D1. `buyer_id` is a nullable VARCHAR appended to the schema
Appended last in `OrderFactsSchema` so a migrated table and a fresh table have the same column order. An empty
`buyer_id` in an event is stored as NULL, never as `''`, so "unattributed" has exactly one representation.
Alternative: `NOT NULL DEFAULT ''` was rejected: DuckDB `ADD COLUMN` with a default on a populated table is
fine, but `''` would later collide with a real id check and hides the difference between "unknown" and "none".

### D2. Old rows stay NULL; the feature ignores NULL buyers
A NULL buyer matches no `user_key`, so it counts for nobody. All seller and platform aggregates are unchanged
(they never read the column).

### D3. BigQuery: evolve the order_facts table too
The BigQuery `Ensure` path only creates `order_facts` when absent. It gets the same additive-column step as the
tracking table (generalise `evolveSchema` to take a table), so an existing prod table gains `buyer_id`. Unit-tested
with the schema helper, not against BigQuery (no credentials locally).

### D4. `user_activity@v2`, v1 retired
Changing a definition without a version bump fails the lock, so the new feature is a new version. Both versions
cannot share `fs:user_activity:current` sensibly (last writer wins), and no consumer reads v1 (grep: only the job's own
e2e), so v1 is removed from `features.yaml` and the lock. Old `fs:user_activity:v1:*` keys expire by TTL.
`fs:user_activity:current` becomes `"2"`. The existing fsm e2e step texts that name `user_activity@v1` or `"1"`
need a version bump (integrator; listed in the report).

### D5. Definition of `paid_orders_30d`
`count(DISTINCT order_id)` over `orders` with `status = 'PAID'`, `buyer_id = user_key`, and
`occurred_at > AS_OF - 30 days AND occurred_at <= AS_OF` (same half-open window shape as the 7-day features). Distinct
because an order is several rows. A user with paid orders but no events or facts still gets a row (the key set is
extended with buyers), with zero for the other features. Value 0 when the user has no orders in the window.

### D6. The input contract fails loudly
`inputs.connect` needs `buyer_id` in `order_facts.parquet`; a file without it raises `ConfigError` (exit 2) naming the
column, instead of silently computing zeros. The export is rewritten every cycle, so the window of old-schema files
ends one cycle after team-analytics restarts.

## Risks / Trade-offs

- Refunds/cancels still count → documented non-goal; `order_facts` only holds PAID facts today.
- Rolling deploy: featurestore before analytics → job exits 2 until the next export (D6). Deploy analytics first.
- New personal-data column in the warehouse → legal review deferred by instruction.

## Migration Plan

Deploy team-analytics (column added on open, BigQuery evolved on start), wait one export cycle, then run the new
featurestore image. Rollback: the column is additive; older code ignores it. Re-lock is part of the featurestore commit.
