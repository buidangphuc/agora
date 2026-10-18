# 00-spec — Order lifecycle after checkout (receipt, expiry, return window, seller work)

initiative: q10-order-lifecycle

Source: backlog Q10, Q20, Q9 and the cancel-UX part of Q12 (`plans/pm-review-2026-10/BACKLOG.md`); evidence `buyer-journey.md` #4, #6, #9, `trust-ops.md` #7, `seller-success.md` #4.
Base: `main` (Q1–Q5 merged). Builds on Q3 audit, Q5 flash-stock holds (`ReleaseFlashSaleStock` returns committed units) and the team-order reservation sweeper.

## Goal
An order never stalls after checkout. An online-pay order that is not paid within 24 hours is reminded 2 hours before its deadline, then cancelled by the system with its domain stock, voucher hold and flash-sale units released; until then the buyer can retry payment ("Thanh toán lại"). A shipped order is completed by the buyer ("Đã nhận được hàng") or by the system 7 days after shipping, which opens reviews and a 15-day return window. A return the seller does not answer within 3 days is approved by the system. Sellers are notified when they have work (new order, return requested, low stock) and see a ship-by deadline 2 days after payment. The cancel button appears only where the backend allows a cancel and sends a reason code. Every completion emits one `OrderCompleted` event that Q7 (referral reward) and q6-trust-enforcement (dispute escalation) will consume.

All durations and the low-stock threshold below are env config; the values given are the defaults decided on 2026-10-16.

## User stories
1. **Unpaid order expires.**
   - Given an online-pay order (payment method not COD) in PENDING, placed at T0, with payment deadline `payment_due_at = T0 + 24h`.
   - When the time reaches `payment_due_at − 2h`, Then one `OrderPaymentReminder` event is emitted and the buyer gets one notification linking to `/checkout/pay/{id}`.
   - When the time passes `payment_due_at` and the order is still PENDING, Then the system cancels it (Pending → Cancelled, actor system, reason `PAYMENT_EXPIRED`), releases its domain reservations, voucher hold and flash-sale units exactly once (same code path as a buyer cancel), emits `OrderCancelled` with that reason, and the buyer is notified.
   - Edge cases:
     - A COD order (Pending, never paid online) has no deadline and is never expired.
     - A payment that settles after the expiry cancel leaves the order Cancelled, credits no seller, and the buyer's payment is refunded in full once.
     - An order paid before the deadline is never reminded again or cancelled.
2. **Retry payment.**
   - Given a PENDING online-pay order before its deadline whose transaction is FAILED or missing.
   - When the buyer taps "Thanh toán lại" (order list, order detail or pay page).
   - Then the pay page opens a transaction (`CreatePayment` when none exists, otherwise it returns the existing FAILED one) and a successful `ProcessMockPayment` (which re-settles PENDING or FAILED) moves the order to PAID. After expiry the pay page shows "Đơn hàng đã hết hạn thanh toán" and no pay action.
3. **Buyer confirms receipt; auto-complete.**
   - Given a SHIPPED order.
   - When its buyer calls `ConfirmReceipt`, Then it becomes COMPLETED (actor buyer), `completed_at` is set, one `OrderCompleted` (completed_by BUYER) is emitted, and the buyer is notified with a review prompt.
   - When `shipped_at + 7 days` passes without a confirm, Then the system completes it (completed_by SYSTEM) the same way.
   - Edge cases:
     - A shipped order with a non-terminal return (PENDING or APPROVED) is not auto-completed until that return is terminal.
     - Confirm by anyone but the buyer is `PERMISSION_DENIED`; confirm of a non-SHIPPED order is `FAILED_PRECONDITION`; concurrent confirm and auto-complete emit one `OrderCompleted`.
     - The seller's existing Shipped → Completed path keeps working and also emits `OrderCompleted` (completed_by SELLER).
4. **Return window.**
   - Given an order in SHIPPED, or COMPLETED with `now < return_window_ends_at = completed_at + 15 days` (one value for all categories).
   - When the buyer creates a return, Then it is accepted (today's cap rules unchanged) and one `ReturnRequested` event is emitted for the seller notification.
   - Edge cases: a return on a PAID (not shipped) order is refused (`FAILED_PRECONDITION`, the buyer cancels instead); a return after the window is refused with "return window closed".
5. **Seller no-answer on a return; escalation hook.**
   - Given a return in PENDING created at R0.
   - When `R0 + 3 days` passes with no seller answer, Then the system moves it PENDING → APPROVED (actor system); the refund step stays the existing APPROVED → REFUNDED path that emits `ReturnRefunded`.
   - Every `OrderReturn` read carries `can_escalate`, computed by the server: true when the return is REJECTED and the order is SHIPPED or still inside its return window. q6-trust-enforcement's dispute slices read it; this initiative builds no dispute flow.
   - Edge cases: a seller answer that races the auto-approve wins or loses by compare-and-set (one transition); a REJECTED or APPROVED return is never auto-approved.
6. **Seller work notifications and ship-by.**
   - Given a seller.
   - When one of their orders is paid (online) or placed (COD), Then they get one notification "Đơn hàng mới" linking to `/seller/orders/{id}`, and the order carries `ship_by = (paid_at or created_at) + 2 days`.
   - When a buyer requests a return, Then they get one notification linking to the returns tab.
   - When a listing's total stock falls from above 5 to 5 units or fewer, Then they get one low-stock notification; it fires again only after stock has risen above 5 and falls again (once per downward crossing).
   - The `/seller` dashboard shows a "Cần gửi" count of PAID (and COD PENDING) orders with overdue `ship_by` highlighted.
7. **Cancel UX matches the backend.**
   - Given an order detail page.
   - Then cancel is offered only for PENDING and PAID; a SHIPPED order offers "Yêu cầu trả hàng" instead. The cancel sends a `CancelReason` code alongside the label; the code is stored on the order and carried on `OrderCancelled`.

## Non-goals
- Dispute escalation + dispute refund → q6-trust-enforcement (builds after this initiative).
- Multi-shop single payment, payment deadlines per checkout instead of per order (Q11); delivery ETA and address integrity (Q12 rest, in q11-checkout-coherence).
- A carrier "delivered" signal: no code writes `ShipmentStatusDelivered`, so timers start at `shipped_at`.
- Per-category return windows, auto-cancel of orders not shipped by `ship_by`, a seller-health card (late-shipment / cancellation / return rates), auto-refund of an APPROVED return the seller never refunds.
- Referral reward itself (Q7) — only the `OrderCompleted` event it will consume.
- Notification preferences for the new types, email/push channels.

## Architecture decisions
- team-order owns every timer: one lifecycle sweep, next to the reservation sweeper (`cmd/server` ticker, `LIFECYCLE_SWEEP_INTERVAL`), runs four passes — reminder, payment expiry, auto-complete, return auto-approve — each a compare-and-set (`orderTransitions` / `TransitionReturn`), each event written by the outbox in the same transaction — Why: the order is the aggregate; mirrors the existing sweeper and outbox (ADR-0002, ADR-0013); idempotent on repeat.
- Deadlines are snapshotted on the order (`payment_due_at` at placement, `ship_by` at payment or COD placement, `auto_complete_at` at shipping, `return_window_ends_at` at completion) from env config read at that moment (`PAYMENT_DEADLINE=24h`, `PAYMENT_REMINDER_BEFORE=2h`, `AUTO_COMPLETE_AFTER=168h`, `RETURN_WINDOW=360h`, `RETURN_AUTO_APPROVE_AFTER=72h`, `SHIP_BY_AFTER=48h`) — Why: a config change never moves a promise already shown to a buyer or seller.
- Transition table gains: Pending → Cancelled (system, expiry), Shipped → Completed (buyer, system; seller kept); return PENDING → APPROVED gains actor system — Why: `transitions.go` is the single source of truth (spec order-lifecycle-guards).
- System expiry cancel reuses `CancelOrder`'s claim-then-release path (domain reservations, voucher, flash holds incl. Q5 committed units) — Why: one release path, already exactly-once and swept on failure.
- Return auto-approve moves no money (APPROVED only), so it is not an audit-first money action; the existing seller APPROVED → REFUNDED path still triggers the refund — Why: keeps the Q3 audit rule scoped to money movements.
- team-payment voids a PENDING/FAILED transaction when it consumes `OrderCancelled` with previous_status PENDING (new `PAYMENT_STATUS_CANCELLED`, refused by `ProcessMockPayment`), and refunds the buyer once if the transaction had settled PAID — Why: today a late settle is "acknowledged without change" in team-order (`consumer/payment.go`), so a buyer could pay for a cancelled order.
- Retry payment needs no new RPC: verified in code, `CreatePayment` returns the existing (FAILED) transaction and `ProcessMockPayment` re-settles PENDING or FAILED — Why: the gap is the pay page 404 when no transaction exists, not the backend.
- Low stock is detected in team-notification from `ListingStockChanged` (team-domain already emits it on every reserve/release/sweep), with the seller resolved from the `ListingChanged` snapshots it already projects; threshold `LOW_STOCK_THRESHOLD=5` — Why: no team-domain change, no new contract.
- `OrderCompleted` carries `completed_by` and `return_window_ends_at`; `OrderReturn.can_escalate` is server-computed — Why: Q7 rewards only after the window closes, and q6's dispute slices check escalation without re-deriving order rules.
- No new dependencies.

## Contracts
This initiative's contract slice merges FIRST across q10/q11: it owns `Order` field numbers 18–24; q11-checkout-coherence uses 25+.
```proto
// platform/order/v1/order.proto — OrderService (additive)
// Buyer of the order only. SHIPPED -> COMPLETED; FAILED_PRECONDITION otherwise.
rpc ConfirmReceipt(ConfirmReceiptRequest) returns (ConfirmReceiptResponse);
message ConfirmReceiptRequest { string id = 1; }
message ConfirmReceiptResponse { Order order = 1; }

enum CancelReason {
  CANCEL_REASON_UNSPECIFIED = 0;
  CANCEL_REASON_CHANGED_MIND = 1;    // FE "changed_mind"
  CANCEL_REASON_WRONG_ITEM = 2;      // FE "wrong_item"
  CANCEL_REASON_DAMAGED = 3;         // FE "damaged"
  CANCEL_REASON_DELIVERY_DELAY = 4;  // FE "delivery_delay"
  CANCEL_REASON_OTHER = 5;
  CANCEL_REASON_PAYMENT_EXPIRED = 6; // system only
  CANCEL_REASON_ADMIN_FORCE_FAIL = 7; // ForceFailSaga only
}
enum CompletedBy {
  COMPLETED_BY_UNSPECIFIED = 0;
  COMPLETED_BY_BUYER = 1;
  COMPLETED_BY_SELLER = 2;
  COMPLETED_BY_SYSTEM = 3;
}
// CancelOrderRequest += (buyer may not send PAYMENT_EXPIRED / ADMIN_FORCE_FAIL)
  CancelReason reason_code = 3;
// Order += (field numbers 18–24 owned by this initiative)
  google.protobuf.Timestamp payment_due_at = 18;        // placed_at + 24h; unset for COD
  google.protobuf.Timestamp ship_by = 19;               // paid_at (or COD created_at) + 2d
  google.protobuf.Timestamp shipped_at = 20;
  google.protobuf.Timestamp completed_at = 21;
  google.protobuf.Timestamp return_window_ends_at = 22; // completed_at + 15d
  CancelReason cancel_reason = 23;
  google.protobuf.Timestamp auto_complete_at = 24;      // shipped_at + 7d; unset otherwise
// OrderReturn += (server-computed at read: REJECTED, and order SHIPPED or inside its return window)
  bool can_escalate = 10;
// OrderCancelled +=
  CancelReason reason_code = 8;
// OrderPaidEvent +=
  string seller_id = 7;
  google.protobuf.Timestamp ship_by = 8;

// order.events (outbox, key = order id, EventEnvelope.type = full name)
message OrderPlaced {            // every placed order; team-notification uses COD ones
  string order_id = 1; string buyer_id = 2; string seller_id = 3;
  platform.payment.v1.PaymentMethod payment_method = 4;
  int64 total_amount = 5; string currency = 6;
  google.protobuf.Timestamp placed_at = 7;
  google.protobuf.Timestamp payment_due_at = 8;  // unset for COD
  google.protobuf.Timestamp ship_by = 9;         // set for COD only
}
message OrderPaymentReminder {   // once per order, at payment_due_at - 2h
  string order_id = 1; string buyer_id = 2; int64 total_amount = 3; string currency = 4;
  google.protobuf.Timestamp payment_due_at = 5;
}
message OrderCompleted {         // once per order; Q7 referral and q6 disputes consume it
  string order_id = 1; string buyer_id = 2; string seller_id = 3;
  int64 total_amount = 4; string currency = 5;
  CompletedBy completed_by = 6;
  google.protobuf.Timestamp completed_at = 7;
  google.protobuf.Timestamp return_window_ends_at = 8;
  bool paid_online = 9;
}
message ReturnRequested {        // once per created return
  string return_id = 1; string order_id = 2; string buyer_id = 3; string seller_id = 4;
  int64 refund_amount = 5; string currency = 6;
  google.protobuf.Timestamp requested_at = 7;
}

// platform/payment/v1/payment.proto — PaymentStatus +=
  PAYMENT_STATUS_CANCELLED = 6;  // order cancelled before payment; ProcessMockPayment refuses it
```
The contract slice refreshes proto copies (e.g. `team-analytics/proto`). No engagement or audit proto change in this initiative.
Existing contracts consumed unchanged: `OrderService.CancelOrder` path, `GetReturnRequest`, `UpdateReturnStatus`, `CreateReturnRequest`, `order.events/ReturnRefunded`, `order.events/OrderShipped`, `PaymentService.CreatePayment`, `ProcessMockPayment`, `GetPayment`, `listing.events/ListingStockChanged`, `ListingChanged`, `FlashSaleService.ReleaseFlashSaleStock`, `VoucherService` release.

## Capabilities → slices
| capability (user-observable) | slice id (= OpenSpec change) | owner | depends_on | uses / provides | repos touched |
|---|---|---|---|---|---|
| Lifecycle contract | q10-order-lifecycle-contract | platform-core | – | provides: all items in Contracts | platform-core (+ proto copies) |
| Unpaid orders are reminded, expire and release stock; cancel reason stored | order-unpaid-expiry | team-order | q10-order-lifecycle-contract | uses: OrderPlaced, OrderPaymentReminder, OrderCancelled.reason_code, Order.payment_due_at/cancel_reason, CancelOrderRequest.reason_code, ReleaseFlashSaleStock, voucher release, domain ReleaseReservation | team-order, platform-e2e |
| A cancelled unpaid order cannot be paid; a late payment is refunded | payment-unpaid-cancel | team-payment | order-unpaid-expiry | uses: PAYMENT_STATUS_CANCELLED, order.events/OrderCancelled | team-payment, platform-e2e |
| Buyer confirms receipt; auto-complete; return window; return auto-approve; ship-by | order-receipt-completion | team-order | order-unpaid-expiry | uses: ConfirmReceipt, OrderCompleted, ReturnRequested, OrderPaidEvent.ship_by/seller_id, Order.ship_by/shipped_at/completed_at/return_window_ends_at/auto_complete_at, OrderReturn.can_escalate | team-order, team-gateway, platform-e2e |
| Sellers and buyers are told about lifecycle work | notification-order-work | team-notification | order-receipt-completion | uses: OrderPlaced, OrderPaidEvent, OrderPaymentReminder, OrderCancelled, OrderCompleted, ReturnRequested, ReturnRefunded, ListingStockChanged, ListingChanged | team-notification, platform-e2e |
| Buyer and seller UI for the lifecycle | frontend-order-lifecycle | team-frontend | payment-unpaid-cancel, order-receipt-completion, notification-order-work | uses: ConfirmReceipt, CancelOrder.reason_code, CreateReturnRequest, CreatePayment, ProcessMockPayment, GetPayment, Order new fields | team-frontend, platform-e2e |

Hot files edited by more than one slice (initiative-build serialises those slices):
- `team-order/internal/service/transitions.go`, `service/order.go`, `service/saga.go` (sweep), `cmd/server/main.go`, `internal/repository/order.go`, `internal/repository/return.go`, `internal/handler/order.go`, `internal/events/publisher.go`, `migrations/0008+` — order-unpaid-expiry → order-receipt-completion (in that order; migration numbers follow it).
- `team-frontend/src/features/order/OrderDetailView.tsx`, `OrderActions.tsx`, `src/app/(checkout)/checkout/pay/[id]/page.tsx` — frontend-order-lifecycle only (one slice), but also edited by q11-checkout-coherence.
- `platform-e2e/compose/order-inventory.override.yaml` (short lifecycle timers), `tests/e2e/support/` order helpers — every backend slice.

## Out of initiative
- Dispute escalation + dispute refund → q6-trust-enforcement (builds after this initiative): `DisputeRuling`, `return_id` on disputes, `ListOrderDisputes`, `DisputeResolved`, the team-order dispute-refund consumer and the `return.dispute_refund` audit event all move there. They consume `can_escalate`, `ReturnRequested`, `OrderCompleted` and the return window from this initiative.
- NEEDS_HUMAN follow-up: a banned seller's open paid orders (from q6 Q8) are not cancelled or refunded by anything yet; no slice in q10 or q6 owns it.
- Build order across initiatives: q10 builds in parallel with q6-trust-enforcement, except q6's dispute slices, which wait for q10; q11-checkout-coherence builds after q10 (shares `team-order/internal/service/order.go` and the pay page).
- Pay page text "Thanh toán không thành công nên hệ thống đã hoàn tác … đơn hàng đã bị hủy" contradicts the backend (a FAILED payment leaves the order PENDING); corrected inside frontend-order-lifecycle as a task.
- `team-notification/FEATURES.yaml` and `AGENTS.md` service table text follow the slices (docs, not requirements).
- team-analytics ignores the new event types (it switches on `type`); no change unless a spec author finds a failing switch.

## NFR and data
- NFR: each sweep pass is bounded (batch 100, like `SweepExpiredReservations`) and idempotent; a reminder, expiry, auto-complete or auto-approve fires within one sweep interval (default 1 min) of its time. Timers, windows and the threshold are env config with the defaults above; e2e overlays shorten them (seconds).
- Money: int64 VND; no new money path — refunds reuse `ReturnRefunded` and the `OrderCancelled` refund in team-payment, each once per id.
- Personal data: none new. Events carry ids, amounts and timestamps only; notifications show order id and amount, no address.
- Risk routing expected: proto (contract), payment (expiry refund, PAYMENT_STATUS_CANCELLED), auth (ConfirmReceipt buyer-only, CancelOrder buyer may not send system reason codes).

## Conventions
- Mirror: compare-and-set + outbox `UpdateOrderStatusFrom` (`team-order/internal/repository/order.go`), `TransitionReturn` (`internal/repository/return.go`), `CancelOrder` release path, `SweepExpiredReservations` and `runReservationSweeper`, `consumer/payment.go` (dedupe ledger, DLQ, `ErrPermanent`).
- team-notification: `consumer/user_events.go` `OrderConsumer` (switch on type, `apply` dedupe), `consumer/listing.go` stock state store.
- E2E: extend `platform-e2e/tests/e2e/features/` order, rma and payment features; timers shortened via the compose overlay, never by sleeping minutes.
- Do not touch: team-domain, team-promotion (consumed unchanged), team-referral (Q7), team-engagement (q6).

## Locked
2026-10-16 — Confirmed by Bùi Đăng Phúc (recommended defaults accepted). 24h pay deadline (T-2h reminder), 7d auto-complete, 15d return window, 3d return auto-approve, 2d ship-by, low stock 5; disputes moved to q6; contract merges first and owns Order fields 18–24.

## Clarifications — spec-clarify round 1 (2026-10-16, Bùi Đăng Phúc: "approve hết", all recommended answers accepted)
Money / risk
- M1 ProcessMockPayment re-reads the order (team-order GetOrder) before settling; a CANCELLED order refuses settle (closes the CreatePayment-vs-cancel race). Owner: payment-unpaid-cancel.
- M2 An APPROVED return on a COD order (no PaidAt) does not block auto-complete; COD refunds are a follow-up. Owner: order-receipt-completion.
- M3 No auto-refund of an APPROVED return the seller never refunds; follow-up.
- M4 ReserveStock / checkout on a HELD listing is refused FAILED_PRECONDITION. Owner: domain-seller-enforcement-hold (team-domain reservation path).
- M5 Legacy CreateOrder.voucher_code is treated as the platform-voucher slot only; no SHOP auto-routing probe. Owner: order-multishop-checkout.
Trust (q6)
- A8 Every seller content edit of a PUBLISHED listing is re-screened; a flagged edit moves it to PENDING_REVIEW. Status omitted must be distinguishable from DRAFT (fix domainStatus UNSPECIFIED handling).
- A9 Admin REJECT/BAN allowed from PENDING_REVIEW and PUBLISHED; no reversal of REJECTED/BANNED (APPROVE refused).
- A10 Seller DeleteListing on BANNED/HELD → FAILED_PRECONDITION.
- A11 screen_flags are admin-only; the owner sees reason_code + note only.
- A12 An admin's own listing follows the seller path (screen runs), still audited.
- A13 ListingChanged events carry no moderation note and no screen flags.
- A14 SetUserStatus RESTRICTED/SUSPENDED/BANNED (including re-setting the same status) revokes all sessions; ACTIVE revokes none. Contract and identity slice must say the same.
- A15 Expired suspension is lifted only at the user's next login (listings stay HELD until then); a lift sweeper is a follow-up.
- A16 Login refusal is one generic message ("tài khoản bị khóa"); no suspended/banned distinction or end time.
- A17 Dispute state machine allows OPEN→INVESTIGATING and OPEN→REJECTED (spam); INVESTIGATING→{RESOLVED,REJECTED}. A ruling with any status other than RESOLVED is INVALID_ARGUMENT (contract comment changes to match).
- A18 Re-escalation of a return after its dispute closed is allowed while can_escalate is true. ListOrderDisputes is allowed to the order's buyer and seller (and admin).
- A19 Max 20 OPEN reports per reporter (RESOURCE_EXHAUSTED); ResolveReport note required, 1–500 chars.
- A20 A restricted seller visiting seller pages sees a "Tài khoản bị hạn chế" notice instead of a /login redirect loop. Owner: frontend-user-enforcement.
Order lifecycle (q10)
- B7 Pre-deploy orders: unpaid orders get no payment_due_at (never auto-expired); backfill SHIPPED auto_complete_at = migration+7d, COMPLETED return_window_ends_at = updated_at+15d, PENDING returns auto_approve_at = migration+3d (fixed values, stated in the proposal).
- B8 ConfirmReceipt is allowed while the buyer's return is PENDING or APPROVED.
- B9 A buyer sending a system-only cancel reason gets PERMISSION_DENIED.
- B10 Buyer notifications: ReturnRefunded yes; OrderCancelled only when reason is PAYMENT_EXPIRED; OrderCompleted always.
Checkout (q11)
- C8 VoucherQuote.scope uses a local enum in order.proto (no import of promotion.proto, no extra proto copies).
- C9 A buyer with addresses but no default and an empty address_id → FAILED_PRECONDITION "address not found" (no first-address fallback).
- C10 Cart default selection = all ticked; on a partial selection /cart hides the voucher row and says "Giảm giá được tính ở bước thanh toán".
Cross-slice fixes (orchestrator)
- q6 engagement proto: q10 no longer touches engagement.proto — the "coordinate field numbers with q10" note is moot; q6-trust-contract owns them.
- q10 contract test must NOT assert "no Order field >= 25" (q11 uses 25–27).
- q6 ownership: duplicate scenario titles between engagement-listing-reports / frontend-listing-moderation / frontend-user-enforcement must be made unique (prefix each with its slice's capability, e.g. "Moderation UI: …", "Users UI: …").
- Every frontend-called RPC must have a gateway forwarder in a named slice (PreviewCheckout → order-multishop-checkout; ConfirmReceipt → order-receipt-completion; AccountAdminService → identity-user-enforcement; ListOrderDisputes/ListDisputes → engagement-dispute-escalation).
- The mock screen default must ALLOW clean text so existing e2e seeds that publish listings still auto-publish.
NEEDS_HUMAN before release: DPO (enforcement record basis, 2-year report retention, Langfuse traces when a real LLM is on, dispute audit metadata); legal sign-off on the prohibited-term list; Kafka ACL on identity.events.
