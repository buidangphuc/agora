# LOCK — q5-advertised-price
status: approved
approved-by: Bùi Đăng Phúc
date: 2026-10-16
spec: plans/q5-advertised-price/00-spec.md (Locked 2026-10-16; hold lifecycle decision amended by spec-clarify Round 1, human, 2026-10-16)
## Slices
| slice | owner | depends_on | rounds | validate | spec_lint | spec-quality | decide (spec) | digest | approve |
|---|---|---|---|---|---|---|---|---|---|
| q5-flash-stock-contract | platform-core | [] | 3 | pass | pass | pass (r2; r3 wording only) | ENGINE_DOWN (LLM fallback) | dfb13830f7ee | [x] |
| promotion-flash-stock-hold | team-promotion | [q5-flash-stock-contract] | 3 | pass | pass | pass (r2; r3 purge scope + NITs) | ENGINE_DOWN (LLM fallback) | df79f26f03f9 | [x] |
| order-advertised-price | team-order | [promotion-flash-stock-hold] | 3 | pass | pass | pass (r2; r3 Phase B commit order) | ENGINE_DOWN (LLM fallback) | da6e80314d62 | [x] |
| frontend-advertised-price | team-frontend | [order-advertised-price] | 2 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | 7d8ccbaccd87 | [x] |
## Build order (cross_change_check)
1. q5-flash-stock-contract (sequential, alone)  2. promotion-flash-stock-hold  3. order-advertised-price  4. frontend-advertised-price — strictly sequential (each depends on the previous); hot files: platform-e2e flash-sale seed/helpers in `tests/e2e/support/` (promotion first, order/frontend reuse).
## Cross-change check
pass (cross_change_check: OK)
## Open issues / NEEDS_HUMAN
- Routing: proto (contract), auth (service-only hold RPCs, scope `promotion.reserve` on `service-team-order`), payment-adjacent (order totals, flash price charged), pii (frontend reads the default-address city for the PDP fee).
- Decisions locked in by spec-clarify (human): hold TTL 15 min, commit in checkout Phase B (flash commit before the team-domain commit), Release returns committed units, cancel frees flash stock; FE threshold constants (500.000đ, 20.000đ–35.000đ) guarded by an e2e parity scenario; checkout shows the error page when CalculateShippingFee fails.
- Defaults accepted by the orchestrator (confirm or override): reason set `not_active`/`price_changed`/`insufficient_stock` checked in that order; replay of a released/expired hold = `not_active`; fresh order ids per checkout attempt; Reserve replay returns the stored outcome and ignores other fields; `calculateShippingFee` timeout 2000 ms (no shared value exists).
- Accepted risks: placed but unpaid orders count as sold flash stock until cancelled (unpaid-order expiry = backlog Q20); any cart price vs current price difference (incl. a seller raising the regular price) refuses checkout; one extra `GetListing` per line at checkout.
- DPO: retention of released/expired hold rows (purged 30 days after terminal; committed rows kept with the campaign); `reservation_id` indirectly linked to a buyer; city read per request for the PDP fee (not stored, not logged).
- team-order's `promotion.proto` copy was a stale subset; the contract slice replaces it with the byte-identical source (regenerated code gains SubscriptionService and SponsoredService clients).
- E2E route for service-only RPCs: `grpcurl` from a throwaway container on the stack network (unconfirmed image availability); flash kill-switch OFF covered by unit tests only.
