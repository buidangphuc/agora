# feature-run identity-session-refresh
initiative: q1-session-continuity | base: 7ca74ab3 | worktree: .claude/worktrees/identity-session-refresh | branch: asdlc/identity-session-refresh | started: 2026-10-14T10:23:28Z
## Preflight   validate=p spec_lint=p
## E2E         author=done run=red as expected (16 scenarios: no refreshToken / 501 unimplemented) files=5 commit=7339628a
## Tasks
| task | scenarios | status | rounds | T0 | red_first | routed | task-reviewer | specialists |
|---|---|---|---|---|---|---|---|---|
| G1 (1-2 migration+repo) | tasks.md 1-2 | todo | 0 | 7339628a | | | | |
| G2 (3 service) | tasks.md 3 | todo | 0 | | | | | |
| G3 (4-6 handlers+authz) | tasks.md 4-6 | todo | 0 | | | | | |
| G4 (7-8 gateway) | tasks.md 7-8 | todo | 0 | | | | | |
## Gates
## Escalations
- FEATURES flip planned→automated (+covered_by auth/session_refresh.feature::<scenario>) to do after green (e2e-author hard rule).
