# Design

## Clarifications

### Round 1 — 2026-10-16

- M2 [money] An approved return on a cash-on-delivery order does not block auto-complete: does an APPROVED return on a COD order (no paid time, so it can never reach REFUNDED) block auto-complete? → **No. An APPROVED return blocks only an order paid online; on a COD order it does not block, and COD refunds are a follow-up** (human)
- B7 [data] Migration backfills deadlines without completing anything: what do pre-deploy orders and returns get? → **Fixed values stated in the proposal: SHIPPED `auto_complete_at` = migration + 7d, COMPLETED `return_window_ends_at` = `updated_at` + 15d, PENDING returns `auto_approve_at` = migration + 3d; unpaid orders get no `payment_due_at`** (human)
- B8 [permission] Buyer may confirm while their return is open: may the buyer call `ConfirmReceipt` while their return is PENDING or APPROVED? → **Yes, allowed** (human)
- X1 [contract] Confirm is forwarded by the gateway: which slice owns the gateway forwarder for the frontend-called `ConfirmReceipt`? → **order-receipt-completion (task 10)** (human)

Review round 1 fixes (no human question): every move to COMPLETED clears `auto_complete_at` in the same CAS (scenario "Seller completion clears the auto-complete deadline"); the e2e `RETURN_AUTO_APPROVE_AFTER` is 30s, above `AUTO_COMPLETE_AFTER=12s`, so "An open return blocks auto-complete" sees a PENDING return at the deadline.
