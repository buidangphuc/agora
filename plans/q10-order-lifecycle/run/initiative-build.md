# initiative-build q10-order-lifecycle
integration: .claude/worktrees/q10-order-lifecycle (asdlc/q10-order-lifecycle, base origin/main @ 2c131915)
LOCK: approved 2026-10-16, digests verified. validate_plan: clean (6 packets, 5 waves, no same-wave overlap, no slice moved).

| wave | slices | base | status |
|---|---|---|---|
| 0 | q10-order-lifecycle-contract | asdlc/q10-order-lifecycle | pending |
| 1 | order-unpaid-expiry | asdlc/q10-order-lifecycle | pending |
| 2 | payment-unpaid-cancel, order-receipt-completion | asdlc/q10-order-lifecycle | pending |
| 3 | notification-order-work | asdlc/q10-order-lifecycle | pending |
| 4 | frontend-order-lifecycle | asdlc/q10-order-lifecycle | pending |

Held until q10-order-lifecycle merges to main: (none). Cross-initiative: q11-checkout-coherence and the q6 dispute slices build after this initiative merges (LOCK.md open issues).
Wave 2 is parallel (team-payment vs team-order/team-gateway, disjoint e2e files); team-order hot files are serialised by the wave order (order-unpaid-expiry before order-receipt-completion).
