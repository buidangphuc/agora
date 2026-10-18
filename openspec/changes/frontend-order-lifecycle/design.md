# Design

## Context

The backend slices of q10-order-lifecycle make every lifecycle step a server decision (24 h payment deadline, buyer or 7-day completion, 15-day return window, 2-day ship-by). The frontend only needs to show those decisions, offer the actions the server allows, and stay correct at the boundaries. `OrderDetailView.tsx`, `OrderActions.tsx` and the pay page are also edited by q11-checkout-coherence, which builds after this initiative, so this slice keeps its edits inside the functions named in tasks.

## Decisions

- **One rules module, pure and clock-injected** (`lifecycle.ts`): the rules "may cancel", "may pay again", "may return" exist today as three scattered predicates (`canCancelFromDetail`, `canCancelFromList`, `canReturnOrder`) and one inline test (`isOnlinePending` in `OrderList`). Moving them next to each other with an explicit `now` makes the boundary scenarios unit-testable (`vi.setSystemTime`) and stops the list and detail from disagreeing. Alternative (ask the server "can I cancel?") rejected: no such RPC and no new RPC is allowed in this initiative.
- **Deadlines as ISO strings in `ViewOrder`**, display formatting separate. Existing `createdAt` / `paidAt` are already `toLocaleString` text and cannot be compared; the new fields are needed for comparison, so they are ISO strings. The old two are left alone.
- **Retry needs no new RPC.** Verified in the initiative spec: `CreatePayment` returns the existing FAILED transaction; `ProcessMockPayment` re-settles PENDING or FAILED. The gap is the pay page 404 when no transaction exists, so the server component creates one. Creating inside a GET render is a side effect, accepted because the call is idempotent per order and a second concurrent render gets the same transaction; the alternative (a client-side "start payment" click) adds a step for the buyer without changing safety.
- **Single label "Thanh toán lại"** on list, detail and pay page for any PENDING online order before its deadline. The list does not load transactions (one extra call per order), so it cannot tell a fresh PENDING transaction from a FAILED one; the pay page, which has the transaction, decides what to render. "Thanh toán ngay" disappears.
- **Cancel reason mapped on the server.** The client sends the option key; `cancelReasons.ts` (no `use client`) is imported by the server action and by `OrderActions`, so label and code never drift.
- **KPI "Cần gửi" replaces "Đơn chờ xử lý"** rather than adding a fifth cell: the old KPI counted unpaid online orders (not actionable by the seller) and the KPI grid is a 4-up row. Overdue is `ship_by < now`; an order exactly at `ship_by` is not yet overdue.
- **System-expiry wording** differs from buyer cancel wording so the buyer understands why; the code comes from `Order.cancel_reason`, not from string matching.

## Risks / trade-offs

- Legacy orders created before the backend deploy have no `payment_due_at`, `ship_by` or `return_window_ends_at`: the UI treats a missing deadline as "no deadline" (pay offered, no deadline text), a missing `ship_by` as "-", and a COMPLETED order without a window as returnable (server decides). Per B7 the backend leaves unpaid legacy orders without `payment_due_at` (never auto-expired) and backfills `auto_complete_at` on SHIPPED and `return_window_ends_at` on COMPLETED orders, so the fallbacks mainly matter for unpaid legacy orders.
- The refusal wording `return window closed` is matched by substring on `rawMessage` (the pattern used for `price changed` in Q5); a different server string falls back to the generic message with no loss of safety.
- The pay page renders a paid or cancelled order from the order alone (no transaction read). This drops the old "success" simulator phase on a reload of a paid order, which showed the same fact.

## Clarifications

### Round 1 — 2026-10-16
- B7 [data] Order lifecycle: an order with no deadline keeps its pay action: Do pre-deploy orders get backfilled deadlines? → **Unpaid orders get no payment_due_at (never auto-expired); SHIPPED get auto_complete_at = migration+7d, COMPLETED get return_window_ends_at = updated_at+15d; the UI keeps "missing deadline = no deadline, server decides"** (human)
- B8 [state] Order lifecycle: a shipped order with an open return still offers receipt: Is ConfirmReceipt allowed while a return is open? → **Yes, while the buyer's return is PENDING or APPROVED; the button stays** (human)
- B9 [permission] Order lifecycle: an unknown cancel reason is refused before any call: What does the backend answer a buyer sending a system-only cancel reason? → **PERMISSION_DENIED; the UI whitelist still refuses it before any call** (human)
- R1 [state] Order lifecycle: the pay page of a non-pending order makes no payment call: What does the pay page render for PAID, SHIPPED, COMPLETED and non-expiry CANCELLED orders? → **A paid Result or the cancelled Result from the order alone, no getPayment or createPayment, no 404** (human)
- R2 [permission] Order lifecycle: a session without seller scope sees no seller orders: Does a session without listing.write get redirected to /login? → **No; the seller layout's "Cần tài khoản Người Bán" screen is the real behavior, and no ListSellerOrders call is made** (human)
- R3 [side-effect] Order lifecycle: the pay page opens a transaction when none exists: Is creating a transaction while rendering the pay page acceptable? → **Yes; CreatePayment is idempotent per order and runs only after the order read succeeds** (human)
- X1 [contract] Order lifecycle: the buyer confirms receipt and the review prompt opens: Who owns the gateway ConfirmReceipt forwarder? → **order-receipt-completion; this slice adds no gateway code** (human)
- M1 [state] Order lifecycle: a payment refused after expiry refreshes the page: Can a payment settle on an order cancelled meanwhile? → **No; ProcessMockPayment re-reads the order and refuses a CANCELLED one (payment-unpaid-cancel), and the simulator refreshes into the expiry or cancelled state** (human)
