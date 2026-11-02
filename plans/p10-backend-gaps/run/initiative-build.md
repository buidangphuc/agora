# initiative-build: p10-backend-gaps

- LOCK.md: approved, digests verified by the orchestrator (2026-11-02).
- Integration worktree: .claude/worktrees/p10-backend-gaps, branch asdlc/p10-backend-gaps.
- validate_plan.py: clean (7 packets, 5 waves).

## Waves

| wave | slices | base branch | status |
|---|---|---|---|
| 0 | p10-gaps-contract | asdlc/p10-backend-gaps | pending |
| 1 | gateway-principal-log-fingerprint, identity-sso-handoff | asdlc/p10-backend-gaps | pending |
| 2 | identity-seller-onboarding, promotion-admin-vouchers | asdlc/p10-backend-gaps | pending |
| 3 | seller-onboarding-ui | asdlc/p10-backend-gaps | pending |
| 4 | promotion-admin-campaigns | asdlc/p10-backend-gaps | pending |

## Wave moves (same-wave write-set overlap)

- promotion-admin-vouchers: wave 1 -> 2; overlaps gateway-principal-log-fingerprint on team-gateway/internal/edge/forward.go and forward_test.go (and identity-sso-handoff on fe-platform/docs/gaps.md).
- promotion-admin-campaigns: wave 2 -> 3 -> 4; overlaps promotion-admin-vouchers (team-promotion store/rpc, gateway promotion.go, gaps.md; it also reuses the vouchers admin error mapping), then seller-onboarding-ui on fe-platform/docs/gaps.md.

## Packets

- wave0-task1-p10-gaps-contract.md
- wave1-task1-gateway-principal-log-fingerprint.md
- wave1-task2-identity-sso-handoff.md
- wave2-task1-identity-seller-onboarding.md
- wave2-task2-promotion-admin-vouchers.md
- wave3-task1-seller-onboarding-ui.md
- wave4-task1-promotion-admin-campaigns.md
