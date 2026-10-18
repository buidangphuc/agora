# LOCK — q10-order-lifecycle
status: approved
approved-by: Bùi Đăng Phúc (chat "approve LOCK, build đi")
date: 2026-10-16
spec: plans/q10-order-lifecycle/00-spec.md (Locked 2026-10-16; spec-clarify round 1 decisions appended, human "approve hết" 2026-10-16)
## Slices
| slice | owner | depends_on | rounds | validate | spec_lint | spec-quality | decide (spec) | digest | approve |
|---|---|---|---|---|---|---|---|---|---|
| q10-order-lifecycle-contract | platform-core | [] | 2 | pass | pass | pass (r2) | ENGINE_DOWN (LLM fallback) | 89bb096a2b36 | [x] |
| order-unpaid-expiry | team-order | [q10-order-lifecycle-contract] | 2 | pass | pass | pass (r2) | ENGINE_DOWN (LLM fallback) | eca26df95a83 | [x] |
| payment-unpaid-cancel | team-payment | [order-unpaid-expiry] | 2 | pass | pass | pass (r2) | ENGINE_DOWN (LLM fallback) | 9de68be1d723 | [x] |
| order-receipt-completion | team-order | [order-unpaid-expiry] | 2 | pass | pass | pass (r2) | ENGINE_DOWN (LLM fallback) | af38241b2fca | [x] |
| notification-order-work | team-notification | [order-receipt-completion] | 2 | pass | pass | pass (r2) | ENGINE_DOWN (LLM fallback) | 8f06040505d3 | [x] |
| frontend-order-lifecycle | team-frontend | [payment-unpaid-cancel, order-receipt-completion, notification-order-work] | 3 | pass | pass | pass (r2) | ENGINE_DOWN (LLM fallback) | 83c8c24fb5d6 | [x] |
## Build order (cross_change_check)
q10-order-lifecycle-contract -> order-unpaid-expiry -> order-receipt-completion -> notification-order-work -> payment-unpaid-cancel -> frontend-order-lifecycle
## Cross-change check
pass (cross_change_check: OK)
## Open issues / NEEDS_HUMAN
- Build: first of the three for team-order; q11-checkout-coherence builds after this merges; q6 dispute slices wait for it.
- Contract owns `Order` fields 18–24 (q11 uses 25+).
- Hot files (serialised): team-order `service/transitions.go`, `service/order.go`, `service/saga.go`, `cmd/server/main.go`, `repository/order.go`, `repository/return.go`, `handler/order.go`, `events/publisher.go`, migrations 0008+; platform-e2e `compose/order-inventory.override.yaml` (timer overlay defined once by order-unpaid-expiry) and `tests/e2e/support/`.
- Routing: proto, payment (expiry cancel releases voucher/flash; late-payment refund; ProcessMockPayment re-checks order status before settle, M1), auth (ConfirmReceipt buyer-only, system-only cancel reasons).
- Defaults locked: deadline 24h, reminder T-2h, auto-complete 7d after shipped_at, return window 15d, return auto-approve after 3d (approve only, no auto-refund), ship-by 2d, low stock 5. Backfill per B7.
- Follow-ups: auto-refund of an APPROVED return the seller never refunds (M3); COD return refunds (M2).
- Clarification decisions (M1–M5, A8–A20, B7–B10, C8–C10) are recorded in 00-spec "Clarifications — spec-clarify round 1" and in each change's design.md.
