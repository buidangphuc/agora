# Design

## Decisions

- The escalation lives inside the existing returns tab (`ReturnRequestSection`), one button per return card, because `can_escalate` is a property of a return. `OrderDetailView.tsx` only gains the prop passing and a `Khiếu nại` section slot, which keeps the merge with q10 and q11 edits to that file small.
- Disputes of an order are loaded on the server page next to `listOrderReturns` and passed to `ReturnStateProvider`-like state so a newly created dispute appears without a reload; the button hides once an OPEN or INVESTIGATING dispute with that `return_id` exists (client mirror of "one open dispute per return"; the server `ALREADY_EXISTS` stays the authority).
- Admin actions mirror the server state machine in a pure function `allowedDisputeActions(dispute)` so the UI never offers OPEN to RESOLVED; the server still refuses it.
- Ruling is required by the UI for a return-linked dispute resolved; a dispute without `return_id` sends no ruling.
- Server actions return `ActionResult` (`@/lib/action-result`) and never forward `err.message`; error codes map to the copy table.
- Page guards use `redirectGuestToLogin` (guest) and the 403 `Result` (non-admin), as `admin/audit/page.tsx` does.
- After any successful admin action the panel navigates to `?tab=disputes&dispute=<id>` and renders the dispute through `GetDispute`, so a dispute that leaves the current status filter (OPEN to INVESTIGATING) stays on screen with its new actions.
- The buyer-favour confirmation names the approval and the online-paid refund only; it never promises a refund, because order-dispute-refund stops a COD return at APPROVED.
- A resolve timeout is not treated as "nothing saved": it shows an unknown-state message and revalidates; only `UNAVAILABLE` (audit-first refusal) shows the audit-down text. This follows the spec-quality-reviewer round 1 recommendation and is open for human confirmation.

## Clarifications

### Round 1 — 2026-10-16

- A17 [state machine] Actions offered per status: which dispute transitions may the admin UI offer, and may a ruling go with a non-RESOLVED status? → **OPEN→INVESTIGATING, OPEN→REJECTED (spam), INVESTIGATING→{INVESTIGATING note, RESOLVED, REJECTED}; a ruling is sent only with RESOLVED (anything else is INVALID_ARGUMENT)** (human)
- A18 [re-escalation] A closed dispute does not hide the button forever: may a buyer escalate a return again after its dispute closed, and who may list an order's disputes? → **Yes, while can_escalate is true; ListOrderDisputes is allowed to the order's buyer, seller and admin (seller view not in this slice)** (human)
- M2 [money] Buyer-favour needs confirmation: what does a buyer-favour ruling do for a COD order? → **The return is approved without a refund (COD refunds are a follow-up), so the confirmation says the refund applies only to online-paid orders** (human)
- X1 [cross-slice gateway] Default list is OPEN, 20 per page: which slice adds the gateway forwarders for ListOrderDisputes and ListDisputes? → **engagement-dispute-escalation; this slice makes no gateway change** (human)
- X2 [ownership] Scenario titles: must titles be unique across the initiative? → **Yes; this slice's duplicate "No disputes" titles are renamed "Order has no disputes" and "Admin tab has no disputes"** (human)
