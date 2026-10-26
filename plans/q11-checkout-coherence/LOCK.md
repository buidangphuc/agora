# LOCK — q11-checkout-coherence
status: approved
approved-by: Bùi Đăng Phúc (chat "approve LOCK, build đi")
date: 2026-10-16
spec: plans/q11-checkout-coherence/00-spec.md (Locked 2026-10-16; spec-clarify round 1 decisions appended, human "approve hết" 2026-10-16)
## Slices
| slice | owner | depends_on | rounds | validate | spec_lint | spec-quality | decide (spec) | digest | approve |
|---|---|---|---|---|---|---|---|---|---|
| q11-checkout-contract | platform-core | [] | 2 | pass | pass | pass (r2) | ENGINE_DOWN (LLM fallback) | 2f32a7b9ddfa | [x] |
| promotion-voucher-scope | team-promotion | [q11-checkout-contract] | 2 | pass | pass | pass (r2) | ENGINE_DOWN (LLM fallback) | 6c87a6e27792 | [x] |
| order-address-eta | team-order | [q11-checkout-contract] | 2 | pass | pass | pass (r2) | ENGINE_DOWN (LLM fallback) | 3f639477106d | [x] |
| order-multishop-checkout | team-order | [promotion-voucher-scope, order-address-eta] | 2 | pass | pass | pass (r2) | ENGINE_DOWN (LLM fallback) | 5a297f8d40e5 | [x] |
| frontend-checkout-coherence | team-frontend | [order-multishop-checkout] | 2 | pass | pass | pass (r2) | ENGINE_DOWN (LLM fallback) | b347ee94ff7d | [x] |
| frontend-cart-selection | team-frontend | [frontend-checkout-coherence] | 2 | pass | pass | pass (r2) | ENGINE_DOWN (LLM fallback) | ea8c2af80a65 | [x] |
## Build order (cross_change_check)
q11-checkout-contract -> order-address-eta -> promotion-voucher-scope -> order-multishop-checkout -> frontend-checkout-coherence -> frontend-cart-selection
## Cross-change check
pass (cross_change_check: OK)
## Open issues / NEEDS_HUMAN
- Build: AFTER q10-order-lifecycle merges (base = main then); shares team-order `service/order.go`, `handler/order.go` and the pay page.
- `Order` fields 25–27; re-verify every new field number against platform-core at build time.
- Hot files (serialised): team-order `service/order.go`, `handler/order.go`; team-frontend `(checkout)/checkout/(summary)/page.tsx`, `features/order/checkoutParams.ts`, `PlaceOrderForm.tsx`, `checkout/pay/[id]/page.tsx`; platform-e2e checkout/voucher helpers.
- Routing: proto, payment (one payment per order, platform voucher split, voucher holds), pii (confirmed address snapshot).
- Decisions locked: per-order payment; platform voucher prorated by items_subtotal (remainder to largest group); shop + platform stacking; platform voucher used once any order is paid, released only if all cancelled before payment (applied on q10's system cancel too); mock ETA HCM/HN +2d else +4d; legacy voucher_code = platform slot only (M5); VoucherQuote.scope local enum (C8).
- Clarification decisions (M1–M5, A8–A20, B7–B10, C8–C10) are recorded in 00-spec "Clarifications — spec-clarify round 1" and in each change's design.md.
