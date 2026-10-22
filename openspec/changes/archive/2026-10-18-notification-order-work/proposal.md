## Why

After checkout nobody tells a seller they have work or a buyer that an unpaid order is about to lapse. Today `OrderConsumer` (`team-notification/internal/consumer/user_events.go`) reacts to `OrderShipped` only, and `ListingConsumer` (`consumer/listing.go`) reacts to `ListingChanged` only (price drop, back in stock). The locked initiative (`plans/q10-order-lifecycle/00-spec.md`, 2026-10-16, backlog Q10/Q20/Q9, `trust-ops.md` #7, `seller-success.md` #4) adds lifecycle events in team-order and asks for: a seller notification for a new order, a requested return and low stock; a buyer notification for the payment reminder, the system expiry cancel, completion with a review prompt, and the refund of a return. This slice is the team-notification side. The events and fields come from `q10-order-lifecycle-contract`, produced by `order-unpaid-expiry` and `order-receipt-completion`.

initiative: q10-order-lifecycle

## What Changes

- **order.events consumer** (`OrderConsumer`, same group, same dedupe ledger `processed_events` under consumer `team-notification.order`, same `ErrPermanent`/DLQ discipline). New envelope types, each creating exactly one `NOTIFICATION_TYPE_ORDER` notification through the existing `notify` (user preference gating unchanged):

| EventEnvelope.type | recipient | title / body | link |
|---|---|---|---|
| `OrderPlaced` with `payment_method == PAYMENT_METHOD_COD` | `seller_id` | "Đơn hàng mới" / order id, amount, ship-by date | `/seller/orders/{order_id}` |
| `OrderPlaced`, any other method | nobody (the seller is told at payment) | - | - |
| `OrderPaidEvent` | `seller_id` (new field) | "Đơn hàng mới" / order id, amount, ship-by date | `/seller/orders/{order_id}` |
| `OrderPaymentReminder` | `buyer_id` | "Đơn hàng sắp hết hạn thanh toán" / order id, amount, deadline | `/checkout/pay/{order_id}` |
| `OrderCancelled` with `reason_code == CANCEL_REASON_PAYMENT_EXPIRED` | `buyer_id` | "Đơn hàng đã bị hủy do quá hạn thanh toán" / order id, amount | `/account/orders/{order_id}` |
| `OrderCancelled`, any other reason | nobody (the actor already knows) | - | - |
| `OrderCompleted` (any `completed_by`) | `buyer_id` | "Đơn hàng đã hoàn tất" / review prompt, return window end | `/account/orders/{order_id}` |
| `ReturnRequested` | `seller_id` | "Yêu cầu trả hàng mới" / order id, refund amount | `/seller/orders/{order_id}?tab=returns` |
| `ReturnRefunded` | `buyer_id` | "Yêu cầu trả hàng đã được hoàn tiền" / refund amount | `/account/orders/{order_id}` |

  An event whose recipient id is empty (e.g. an `OrderPaidEvent` published before `seller_id` existed) is skipped with a warning and is not a poison record (no DLQ). A payload that does not unmarshal, or lacks `order_id`, is `ErrPermanent`.
- **listing.events consumer** (`ListingConsumer`, same group and ledger): handles a second type, `ListingStockChanged`, and records seller and title from `ListingChanged`.
  - New table `listing_low_stock_state` (migration `005_low_stock_state`): `listing_id` (PK), `seller_id`, `title`, `low` (bool, true when the last seen total stock `<= LOW_STOCK_THRESHOLD`), `updated_at`. In-memory default store for unit tests, Postgres store injected in `cmd/server/main.go`, like `StockStateStore`.
  - `ListingChanged` (not DELETED) upserts `seller_id`, `title`, and sets `low = snapshot stock <= threshold` on every snapshot (seed on first sight, and re-arm after a seller restock through an edit, which team-domain emits only as `ListingChanged`); it never fires a low-stock notification.
  - `ListingChanged` with `change_type == CHANGE_TYPE_DELETED` deletes the listing's `listing_low_stock_state` row (no notification); a later `ListingStockChanged` for it is then ignored as unknown.
  - `ListingStockChanged`: with a row present, compute `low_now = stock <= threshold`. `low` false and `low_now` true: one notification to `seller_id`, then `low` is set true (advanced only after the notification succeeded, so a redelivery re-detects). `low` true and `low_now` false: `low` is set false (re-armed), no notification. Otherwise nothing. With no row (listing never seen in a `ListingChanged`): ignored with a warning, nothing stored.
  - Notification: `NOTIFICATION_TYPE_SYSTEM`, title "Sắp hết hàng", body "{title}: còn {stock} sản phẩm", link `/seller/{listing_id}/edit` (the seller edit route).
  - `ListingStockChanged.stock` is the listing's total stock (team-domain emits it as the listing total next to per-variant `variants`).
- **Config**: `LOW_STOCK_THRESHOLD` (default 5, read at startup in `internal/config`, injected into the consumer). Topics, groups and DLQs are unchanged (`order.events`, `listing.events`).
- **Proto**: `team-notification/proto/platform/order/v1/order.proto`, `payment/v1/payment.proto` copied from `platform-core/packages/proto` (the contract slice's version), Go stubs regenerated; never hand-edited. No notification proto change (`NotificationType` already has ORDER and SYSTEM).
- **platform-e2e** (owned by this slice): scenarios in `features/notification/order_work.feature` and steps; the overlay `compose/order-inventory.override.yaml` already shortens lifecycle timers (set by the team-order slices); this slice may only add `LOW_STOCK_THRESHOLD` there if the e2e needs another value (default 5 suffices).
- Not changed: team-order, team-domain, team-gateway (`ListNotifications` already routed), the frontend (notification center renders any `link_url`), notification preferences (new types reuse ORDER and SYSTEM), email/push.

## NFR

- Latency: a notification is created in the same handler call as the event is consumed (no upstream call); end to end it appears within one Kafka poll plus the order outbox relay interval of the producing event. One DB write per notification, plus one state read/write for a stock event.
- Availability: a transient failure (DB, prefs read) returns a non-permanent error: the loop retries 5 times with backoff, then DLQs `order.events.dlq` / `listing.events.dlq`, and the offset is committed only after apply or DLQ (existing discipline). Redelivery never duplicates a notification: the dedupe ledger records `event_id` only after the effect succeeded.
- Limits: one notification per (event_id, recipient); body text <= 200 characters; the threshold is `int32 >= 0` (invalid value fails startup).

## Data classification

| Field | Class | Purpose | Basis | Retention |
|---|---|---|---|---|
| order id, buyer id, seller id in a notification | pseudonymous ids | route the notification to its owner | contract performance (order processing) | existing `notifications` retention (unchanged) |
| amount (VND), deadline, ship-by, return window end | operational | message body | same | same |
| listing title and `seller_id` in `listing_low_stock_state` | public listing data + pseudonymous id | name the listing and find its owner | legitimate interest (seller operations) | until the `ListingChanged` DELETED event for the listing (row deleted then); no personal data beyond the seller id |

No address, phone, email or buyer name is read or stored (the `OrderPlaced` and other events carry none).

## Threat model

- **Assets:** a user's notification feed (it reveals an order id and amount); the recipient mapping.
- **Abuse cases:**
  - A buyer or seller sees another user's order notification: the recipient is taken only from the event's `buyer_id` / `seller_id`, never from a client; `ListNotifications` stays scoped to the caller's principal (existing).
  - Spoofed event on `order.events` to spam a user: topics are service-only (broker ACL, existing); the consumer trusts only envelopes from the topic and rejects a malformed one to the DLQ.
  - Duplicate delivery or a broker replay floods a seller: `event_id` dedupe plus the low-stock `low` flag (one notification per downward crossing).
  - A COD order notified twice (once from `OrderPlaced` and again later): only `OrderPlaced` with COD notifies; an online order is notified only from `OrderPaidEvent`.
  - A buyer told "expired" for their own cancel: only `PAYMENT_EXPIRED` notifies.
- **Authz rule:** no new RPC. A notification is created for exactly one user id taken from the event; reading stays `ListNotifications` / `MarkAsRead` scoped by principal.

## Open decisions

- Seller link for low stock uses `/seller/{listing_id}/edit` (existing route in `team-frontend/src/app/(shop)/seller/[id]/edit`).
- A listing with no `ListingChanged` ever seen by this consumer never fires low stock (no seller known); listings created before this deploys fire after their first `ListingChanged`.
