# Design

## Context

`CancelOrder` already claims Cancelled first with a compare-and-set (`UpdateOrderStatusFrom`, which writes the `OrderCancelled` outbox row in the same transaction), then releases reservations, the voucher hold and flash holds. `SweepExpiredReservations` and `runReservationSweeper` already retry parked releases. The expiry reuses all of it; the new parts are the deadline, the two events, the sweep passes and the reason code.

## Decisions

- **One cancel path, three actors.** Buyer cancel, `ForceFailSaga` and the expiry call one `cancelWith(ctx, id, allowedFrom, reason)` that does the claim and the post-claim release. The only differences are the allowed source statuses (buyer: Pending, Paid; system: Pending) and the reason. Why: exactly-once release is already proven on this path (spec order-lifecycle-guards).
- **Deadline snapshot at placement.** `payment_due_at` is stored from `PAYMENT_DEADLINE` at the moment of `PlaceOrders`; a later config change never moves a promise shown to the buyer. The reminder offset `PAYMENT_REMINDER_BEFORE` is NOT snapshotted: it is read at sweep time (it is not a promise).
- **Expiry boundary.** An order is expired when `payment_due_at < now` (strictly after); at `now == payment_due_at` it is still payable. The reminder window is `payment_due_at - PAYMENT_REMINDER_BEFORE <= now < payment_due_at`; a sweep that first sees an order after its deadline (service was down) sends no reminder and goes straight to expiry. Expiry runs before reminder within a tick for that reason. The initiative spec lists the passes as reminder then expiry; that list names the passes, it is not an execution order, and this change's order (expiry first) governs.
- **COD and legacy rows.** COD orders have `payment_due_at` NULL and the sweep also filters on `payment_method`. Orders placed before migration 0008 have NULL and are skipped; no backfill (B7), because stamping a deadline retroactively would cancel orders buyers were never warned about.
- **Reminder is a CAS stamp.** `UPDATE orders SET payment_reminder_sent_at = now() WHERE id = $1 AND status = 1 AND payment_reminder_sent_at IS NULL` plus the outbox insert in one transaction: the stamp is the dedupe, the deterministic event id is the second guard.
- **OrderPlaced for every order**, written in the placement transaction (all-or-nothing with the order row), because team-notification uses COD ones (`order-receipt-completion` fills `ship_by`). This slice owns the emission since it already edits `PlaceOrders` for `payment_due_at`.
- **Reason code semantics.** The order row stores the `CancelReason` that won the claim; `OrderCancelled.reason_code` is built from the row the claim returned, so it can never disagree with the order. Unset from a client is stored as UNSPECIFIED (not rewritten to OTHER) so old clients are distinguishable.
- **Late payment.** team-order's consumer keeps acknowledging a settle for a non-Pending order without change; making the buyer whole is team-payment's job from the `OrderCancelled` event (`payment-unpaid-cancel`). Retry payment needs no team-order change (initiative decision).
- **Sweep shape.** `SweepLifecycle` is one function with ordered, independent passes so `order-receipt-completion` appends passes without touching the ticker; the ticker is separate from the reservation sweeper because the intervals differ in the e2e overlay.

## Risks

- Short e2e deadlines apply to the whole team-order container in the overlay and could cancel unrelated scenarios' unpaid orders; the overlay uses `PAYMENT_DEADLINE=45s`, `PAYMENT_REMINDER_BEFORE=30s`, `LIFECYCLE_SWEEP_INTERVAL=2s`, checked against the existing order features (task 9). E2E scenarios state times relative to these variables, never as fixed hours.
- The Pending to Cancelled system row is added to the guard spec through a MODIFIED delta; `order-receipt-completion` adds the Shipped to Completed rows in its own delta against the same requirement and the two merge in build order.

## Clarifications

### Round 1 — 2026-10-16

- B7 [data / migration] Order expiry: An order without a deadline is never expired: do pre-deploy unpaid orders get a `payment_due_at` backfill? → **No: unpaid orders placed before migration 0008 get no `payment_due_at` and are never auto-expired (the other B7 backfills belong to `order-receipt-completion`)** (human)
- B9 [authz] Order expiry: Buyer cannot send a system reason code: what does a buyer get for a system-only cancel reason (`PAYMENT_EXPIRED`, `ADMIN_FORCE_FAIL`)? → **`PERMISSION_DENIED`, before any write** (human)
- B10 [events] Order expiry: Expiry emits OrderCancelled with the expiry reason: which cancels notify the buyer? → **Only `OrderCancelled` with reason_code `PAYMENT_EXPIRED`; this slice must emit that reason_code on every expiry, the filter lives in `notification-order-work`** (human)
