# initiative-build q6-trust-enforcement
integration: .claude/worktrees/q6-trust-enforcement (asdlc/q6-trust-enforcement, base origin/main)
LOCK: approved 2026-10-16 (Bùi Đăng Phúc), digests verified; packets validated by scripts/validate_plan.py (11 packets, 7 waves, clean).

| wave | slices | base | status |
|---|---|---|---|
| 0 | q6-trust-contract | asdlc/q6-trust-enforcement | merged (branch asdlc/q6-trust-contract, PR 1/2) |
| 1 | ai-listing-screen, engagement-listing-reports | asdlc/q6-trust-enforcement | merged (ai-listing-screen + fixup) |
| 2 | domain-listing-moderation | asdlc/q6-trust-enforcement | merged |
| 3 | frontend-listing-moderation, identity-user-enforcement | asdlc/q6-trust-enforcement | merged |
| 4 | engagement-dispute-escalation, domain-seller-enforcement-hold, frontend-user-enforcement | asdlc/q6-trust-enforcement | engagement-dispute-escalation HELD (q10); domain-seller-enforcement-hold, frontend-user-enforcement merged |
| 5 | order-dispute-refund | asdlc/q6-trust-enforcement | HELD (q10) |
| 6 | frontend-dispute-cases | asdlc/q6-trust-enforcement | HELD (q10) |

## Converge
8 slices merged into asdlc/q6-trust-enforcement. Converge green 2026-10-17: features-check clean, full e2e 67/67 run file by file on the local q2 stack (with opensearch, team-search, team-search-indexer; team-ai in mock mode with the rag-off overlay), feature-reviewer pass per built change. 3 dispute slices held (below); their tasks stay unticked.

## Held slices
engagement-dispute-escalation, order-dispute-refund and frontend-dispute-cases are held until q10-order-lifecycle merges to main (they consume OrderReturn.can_escalate, the return window and GetReturnRequest from order-receipt-completion). Rebase asdlc/q6-trust-enforcement on main after that merge, then build them.

## Wave moves (validate_plan same-wave overlap)
DAG levels moved because of write-set overlap (mostly the hot file team-gateway/internal/edge/policy.go):
- identity-user-enforcement: DAG level 1 -> wave 3 (overlaps engagement-listing-reports in wave 1, domain-listing-moderation in wave 2: policy.go, policy_edge_test.go, unrouted_test.go, team-gateway/generated, FEATURES.yaml).
- engagement-dispute-escalation: DAG level 2 -> wave 4 (overlaps domain-listing-moderation and identity-user-enforcement: policy.go and gateway edge tests). Rule used: the later slice in LOCK build order moves, except that a slice held for q10 always yields to a non-held slice, so identity-user-enforcement and its dependents are not blocked behind the q10 merge.
- Dependents shifted accordingly: domain-seller-enforcement-hold, frontend-user-enforcement (wave 4), order-dispute-refund (5), frontend-dispute-cases (6).
