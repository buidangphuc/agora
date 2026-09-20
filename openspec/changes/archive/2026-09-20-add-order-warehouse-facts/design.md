# Design: Order Events Outbox & Warehouse `order_facts`

## Context
ADR-0013 requires server-authoritative purchase facts to enter the analytics warehouse. Client-side tracking is best-effort and lacks line-item monetization facts.

## Decisions

### 1. Kafka Topic & Event Structure
- Topic: `order.events`
- Key: `order_id`
- Envelope: `platform.events.v1.EventEnvelope` (standardized across Agora)
- Payload: `platform.order.v1.OrderPaidEvent`
  ```protobuf
  message OrderPaidEvent {
    string order_id = 1;
    string buyer_id = 2;
    repeated OrderLineItemFact items = 3;
    int64 total_amount = 4;
    string currency = 5;
    google.protobuf.Timestamp paid_at = 6;
  }

  message OrderLineItemFact {
    string listing_id = 1;
    string variant_id = 2;
    string seller_id = 3;
    int32 quantity = 4;
    int64 unit_price = 5;
    string currency = 6;
  }
  ```

### 2. Transactional Outbox in `team-order`
To avoid dual-write hazards, when an order's status is updated to `PAID` (or `ORDER_STATUS_PAID`), an outbox row is inserted into `order_outbox_events` within the same database transaction. A background relayer queries with `FOR UPDATE SKIP LOCKED`, produces to Kafka, and marks the record `published`.

### 3. Warehouse Separation (`order_facts` vs `tracking_events`)
`team-analytics` maintains a distinct `order_facts` table with columns:
- `event_id` (TEXT, PK / dedupe key)
- `order_id` (TEXT)
- `listing_id` (TEXT)
- `variant_id` (TEXT)
- `seller_id` (TEXT)
- `quantity` (BIGINT)
- `unit_price` (BIGINT)
- `currency` (TEXT)
- `occurred_at` (TIMESTAMP)
- `status` (TEXT)

### 4. Query Rewriting in `team-analytics`
- `RevenueBreakdown`:
  ```sql
  SELECT
    listing_id AS sku,
    SUM(quantity * unit_price) AS revenue,
    SUM(quantity) AS units
  FROM order_facts
  WHERE seller_id = ? AND occurred_at >= ? AND occurred_at <= ?
  GROUP BY listing_id
  ORDER BY revenue DESC
  LIMIT ?
  ```
- `SellerFunnel`:
  Aggregates impressions, views, and adds from `tracking_events`, and joins/queries distinct `order_id` from `order_facts`.
