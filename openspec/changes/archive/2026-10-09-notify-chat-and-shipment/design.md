## Context

- team-chat `internal/events/publisher.go` publishes a `ChatMessage` envelope to `chat.events`
  directly (no outbox) for the gateway's SSE push.
- team-order has an outbox and relayer for `order.events` (`OrderPaidEvent`), with the
  ordered claim query.
- team-notification has a Kafka consumer loop for `listing.events` with a dedupe ledger,
  a DLQ, and per-user ownership (`callerUserID`) on its RPCs.
- team-analytics' order consumer skips any type other than `OrderPaidEvent`.

## Decisions

1. **Recipient of a chat notification** is the thread participant who is not the sender.
   Prefer carrying it in the event (additive field on the published message, or a
   `MessageSent` event with `thread_id`, `sender_id`, `recipient_id`) over team-notification
   calling team-chat per message. The rule is DB-per-service, and an RPC per event adds
   coupling.
2. **Chat publishing stays best-effort** (no outbox), as today. A lost event means a missed
   notification, not lost data; documented as a known limitation.
3. **`OrderShipped` goes through team-order's outbox** in the shipment transaction, like
   `OrderPaidEvent`.
4. **One consumer per topic** in team-notification (`chat.events`, `order.events`), reusing
   the listing consumer's loop, dedupe ledger (keyed by `event_id`) and DLQ handling.
5. **Preferences**: before creating a notification, read the recipient's
   `NotificationPrefs`. A type explicitly set to false is skipped; missing means enabled.
6. **Notification text** (Vietnamese, matching existing copy): chat
   "Tin nhắn mới từ <label>" / first 120 characters; order "Đơn hàng đã được giao cho
   <carrier>" / "Mã vận đơn: <tracking>".

## Risks / Trade-offs

- A chat event lost before Kafka means no notification (best-effort publish).
- The sender label may only be an id until display names are resolved; use the shop display
  name or username if already available in the event, else a generic label.

## Open Questions

None blocking.
