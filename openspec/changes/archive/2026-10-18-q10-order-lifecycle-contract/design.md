# Design

## Clarifications
### Round 1 — 2026-10-16
- Field names, numbers and enum values follow the Contracts section of `plans/q10-order-lifecycle/00-spec.md` verbatim (Locked).
- Enum value names are prefixed (`CANCEL_REASON_*`, `COMPLETED_BY_*`) per buf lint; the FE strings `changed_mind` etc. are documented in comments only.
- `OrderCancelled.reason_code` and `Order.cancel_reason` both use `CancelReason`; the existing free-text `CancelOrderRequest.reason` is kept untouched.
- `PAYMENT_STATUS_CANCELLED = 6` is appended; no existing value moves.
- Copies: `order.proto` in ten repos and `payment.proto` in ten repos, all byte-identical to the source today, are replaced by the new source. team-analytics receives both (the spec's "proto copies, e.g. team-analytics").
- Test location: `platform-core/packages/proto/platform/order/v1/`, dynamic-descriptor approach from the analytics tests (`buf build -o -#format=json`).
- B7 [data-migration] Order lifecycle contract: Derivation rules are documented: what do pre-deploy orders carry? → **unpaid pre-deploy orders get no `payment_due_at` (never auto-expired); backfill of SHIPPED `auto_complete_at` = migration+7d, COMPLETED `return_window_ends_at` = updated_at+15d and PENDING `auto_approve_at` = migration+3d is owned by the behaviour slices; the contract comment says "unset for COD and for orders placed before the deploy"** (human)
- B8 [precondition] Order lifecycle contract: ConfirmReceipt authz and preconditions are documented: may the buyer confirm receipt while a return is open? → **yes, while the buyer's return is PENDING or APPROVED; the RPC comment states it** (human)
- B9 [authz] Order lifecycle contract: System-only cancel reasons are documented as refused for buyers: what does a buyer sending a system-only reason get? → **`PERMISSION_DENIED` (already documented; unchanged)** (human)
- M1 [money] Order lifecycle contract: PAYMENT_STATUS_CANCELLED exists: how is a payment for a cancelled order refused? → **ProcessMockPayment re-reads the order and refuses settle on CANCELLED; the existing "ProcessMockPayment refuses it" comment covers it; enforcement in payment-unpaid-cancel** (human)
- A18 [escalation] Order lifecycle contract: Derivation rules are documented: may a return be re-escalated after its dispute closed? → **yes, while `can_escalate` is true; the `can_escalate` comment states it (not a test assertion)** (human)
- X1 [ownership] Order lifecycle contract: Order lifecycle fields have fixed numbers: may the test assert "no Order field >= 25"? → **no; q11 uses 25–27, the test pins 1–24 only** (human, cross-slice fix)
- X2 [ownership] Order lifecycle contract: Event semantics are documented: does q10 touch engagement.proto? → **no; q6-trust-contract owns those field numbers (this change never touched it)** (human, cross-slice fix)
- X3 [gateway] Order lifecycle contract: ConfirmReceipt RPC exists: who adds the gateway forwarder? → **order-receipt-completion (already out of scope here)** (human, cross-slice fix)

### Reviewer recommendations applied (spec-quality-reviewer round 1, no BLOCKING)
- `payment_due_at` derivation uses `created_at` + 24h (`Order` has no `placed_at`; `placed_at` stays on `OrderPlaced` only).
- `make breaking` compares against HEAD: the compat check runs before the slice commit (HEAD = base from main), the mutation check after it (HEAD = the slice commit).
- Event-id retention stays with order-unpaid-expiry and order-receipt-completion, which must state it.
