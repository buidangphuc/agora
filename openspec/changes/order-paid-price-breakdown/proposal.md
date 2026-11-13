## Why

initiative: monetization

team-payment will book a platform commission when it handles `OrderPaidEvent`, without a synchronous `GetOrder`
(initiative decision: the consumer stays async and replayable). Today the event carries only `total_amount`
(`team-order/internal/events/publisher.go:66`), so team-payment cannot tell items from shipping or a seller voucher
from a platform voucher. The monetization-contract slice adds four optional fields (9-12) to `OrderPaidEvent`;
this slice makes team-order fill them when an order moves to PAID.

## What Changes

- `BuildOrderPaidEnvelope` (and so `BuildPaidOutboxRow`) sets, from the stored order:
  `items_subtotal` = `Order.ItemsSubtotal`, `shipping_fee` = `Order.ShippingFee`,
  `platform_discount_amount` = `Order.PlatformDiscountAmount`,
  `shop_discount_amount` = `Order.DiscountAmount - Order.PlatformDiscountAmount` (`discount_amount` includes the
  platform share, order.proto:404-428). Zero values are set explicitly (proto3 optional presence), so a consumer can
  tell "no discount" from "published before deploy".
- Guard: the breakdown is attached only when it is coherent, that is `items_subtotal + shipping_fee -
  shop_discount_amount - platform_discount_amount == total_amount` and both discount shares are >= 0. Otherwise the
  event is published in the old shape (fields 9-12 absent), the PAID transition is not blocked, and an error log plus
  counter `order_paid_breakdown_skipped_total` record it.
- Fields 1-8, the event id (`OrderPaidEventID`), the envelope type, the outbox row shape and the same-transaction
  enqueue are unchanged. Outbox rows enqueued before deploy are not rewritten.
- e2e: a scenario on the real broker asserts the published payload carries the breakdown matching the order.
- No proto change here (owned by monetization-contract), no migration, no new RPC, no consumer change.

## Capabilities

### New Capabilities
- `order-paid-price-breakdown`: the price breakdown fields on `OrderPaidEvent` as published by team-order.

### Modified Capabilities
none (the existing `order-service-runtime` outbox requirement says "payload bytes SHALL be unchanged" for the
go-sdk migration; that clause is about that migration and is not restated here).

## Impact

- team-order: `internal/events/publisher.go`, its tests, one metric. platform-e2e: one scenario in
  `tests/e2e/features/order/order_events_published.feature` plus steps.
- Event consumers that ignore fields 9-12 (team-analytics `order_facts`, notification, GA4 relay) are unaffected:
  protobuf adds optional fields compatibly.

## NFR

- Latency: building the breakdown is arithmetic on in-memory fields; adds no query and no measurable time to the
  PAID transaction (budget: < 1 ms p95 added to envelope build).
- Availability: a malformed breakdown never blocks or rolls back the PAID transition; the event degrades to the old shape.
- Money: integer VND, int64, no rounding in this slice.
- Compatibility: wire-compatible with old consumers; events published before deploy stay valid and simply lack fields 9-12.

## Data classification

No new personal data. The added fields are order amounts (financial, non-personal on their own); the event already
carries `buyer_id` and `seller_id` for existing purposes (settlement, order facts). Purpose of the new fields:
commission computation in team-payment. Consent basis: contract performance (processing an order the buyer placed).
Retention: unchanged, outbox rows are purged 7 days after publish; the Kafka topic follows its existing retention.
Raw ids are never logged by the guard log line (order id fingerprint only, per sloglint rules).

## Threat model

- Assets: the amounts that drive commission, so the seller's ledger credit and the platform's revenue.
- Abuse cases: (1) a client tampering with the breakdown: none can, the values are read from the stored order row at
  the PAID transition, never from request input; (2) an inflated shop discount lowering commission: the coherence
  guard against `total_amount` makes an inconsistent breakdown unpublishable; (3) a negative shop share (platform
  share larger than `discount_amount`) producing a negative commission base: rejected by the guard; (4) replays:
  the event id is stable per order, so a re-emitted PAID fact collapses to one outbox row.
- Authz rule: no RPC is added or changed; only the PAID transition (already authorized, payment-consumer driven)
  triggers the event. The event is internal to the broker.
