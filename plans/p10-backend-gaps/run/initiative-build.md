# initiative-build: p10-backend-gaps

- LOCK.md: approved, digests verified by the orchestrator (2026-11-02).
- Integration worktree: .claude/worktrees/p10-backend-gaps, branch asdlc/p10-backend-gaps.
- validate_plan.py: clean (7 packets, 5 waves).

## Waves

| wave | slices | base branch | status |
|---|---|---|---|
| 0 | p10-gaps-contract | asdlc/p10-backend-gaps | merged (f8a0397e) |
| 1 | gateway-principal-log-fingerprint | asdlc/p10-backend-gaps | merged (bd18f1b0, re-run merge 67187e85) |
| 2 | identity-sso-handoff | asdlc/p10-backend-gaps | in progress |
| 3 | identity-seller-onboarding, promotion-admin-vouchers | asdlc/p10-backend-gaps | vouchers in progress; seller-onboarding not started |
| 4 | seller-onboarding-ui | asdlc/p10-backend-gaps | pending |
| 5 | promotion-admin-campaigns | asdlc/p10-backend-gaps | pending |

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

## Checkpoint 2026-11-02 (paused by the human)

Stopped workflow wf_b3aec38e-4d6 (continuation script workflows/scripts/build-p10-backend-gaps-cont.js in the session dir). Plain fixes merged (cf7b5fd1). Nothing pushed.

| slice | worktree branch | state | resume at |
|---|---|---|---|
| p10-gaps-contract | asdlc/p10-gaps-contract | merged | - |
| gateway-principal-log-fingerprint | asdlc/gateway-principal-log-fingerprint | merged, e2e 6/6 | - |
| identity-sso-handoff | asdlc/identity-sso-handoff (HEAD 723ef279) | e2e red committed; 1.1.1-1.5.1 approved; 1.6.1 test red committed, implementer stopped mid-run | 1.6.1 implementer; uncommitted package-lock.json and vendor/fe-platform-session.tgz in team-frontend, -seller, -admin are its partial work (rebuilt session tarball), keep or regenerate |
| promotion-admin-vouchers | asdlc/promotion-admin-vouchers (HEAD c93c688f) | e2e red committed; 1.1-1.6 approved | 1.6a: tests c93c688f pass without code (delivered by 1.5/1.6); run the covered-by-earlier review, then 1.7+ |
| identity-seller-onboarding | - | held (needs sso merged) | full slice |
| seller-onboarding-ui | - | held | full slice |
| promotion-admin-campaigns | - | held (needs vouchers + seller-onboarding-ui) | full slice |

Decisions taken during build:
- SSO 1.3.1 (human, 2026-11-02): gateway forwards the verified token sid as trusted metadata x-principal-sid; go-sdk auth.Principal.SessionID; identity CreateHandoffCode binds to it, empty sid -> UNAUTHENTICATED. Write-set widened to go-sdk/pkg/auth and gateway forward.go. The earlier round-3 ListSessions fallback (security weakening) was reverted (942dfc87, 86e33eba).
- Vouchers 1.5: test defect fixed (duplicateOnCreate stubs GetByCode).

Open for the human: promotion admin scope name (`admin` as in proto vs `promotion.admin`).

Resume: edit E2E_DONE / SKIP_TASKS in the continuation script (sso skip through 1.5.1; vouchers skip through 1.6, mark 1.6a covered) and run it fresh (do not use resumeFromRunId: cache replays only a prefix and re-ran merged slices last time). Then converge (initiative-build step 4).
