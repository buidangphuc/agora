# initiative-build q11-checkout-coherence
integration: .claude/worktrees/q11-build (asdlc/q11-checkout-build, base origin/main with q10-order-lifecycle and the q6 slices merged)
LOCK: approved 2026-10-16 (Bùi Đăng Phúc), digests verified 2026-10-22; packets validated by scripts/validate_plan.py (6 packets, 5 waves, clean).

| wave | slices | base | status |
|---|---|---|---|
| 0 | q11-checkout-contract | asdlc/q11-checkout-build | pending |
| 1 | promotion-voucher-scope, order-address-eta | asdlc/q11-checkout-build | pending |
| 2 | order-multishop-checkout | asdlc/q11-checkout-build | pending |
| 3 | frontend-checkout-coherence | asdlc/q11-checkout-build | pending |
| 4 | frontend-cart-selection | asdlc/q11-checkout-build | pending |

## Held slices
(none). q10-order-lifecycle is merged to main, so the LOCK "build after q10" condition is met.

## Wave moves (validate_plan same-wave overlap)
None: waves equal the DAG levels of slices.yaml; wave 1 write-sets (team-promotion vs team-order, distinct e2e files) are disjoint.

## Notes
- Hot files serialised by the DAG: team-order service/order.go, handler/order.go (waves 1 then 2); team-frontend (summary)/page.tsx, checkoutParams, PlaceOrderForm (waves 3 then 4).
- Scenarios that apply SAVE10 call platform-e2e tests/e2e/support/voucher_seed.py ensure_known_voucher.
