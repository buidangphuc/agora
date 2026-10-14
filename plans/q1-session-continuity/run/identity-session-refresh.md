# feature-run identity-session-refresh
initiative: q1-session-continuity | base: 7ca74ab3 | worktree: .claude/worktrees/identity-session-refresh | branch: asdlc/identity-session-refresh | started: 2026-10-14T10:23:28Z
## Preflight   validate=p spec_lint=p
## E2E         author=done run=red as expected (16 scenarios: no refreshToken / 501 unimplemented) files=5 commit=7339628a
## Tasks
| task | scenarios | status | rounds | T0 | red_first | routed | task-reviewer | specialists |
|---|---|---|---|---|---|---|---|---|
| G1 (1-2 migration+repo) | tasks.md 1-2 | approved | 2 | 7339628a | PASS | auth pii | approve | auth-scope PASS (r2: B1 expiry fixed) |
| G2 (3 service) | tasks.md 3 | approved | 2 | ad04e31d | PASS (manual: head tests red@base, green@head; scripted run blocked by BSR rate limit) | auth pii | approve (r2) | auth-scope PASS |
| G3 (4-6 handlers+authz) | tasks.md 4-6 | approved | 1 | 1abcf515 | PASS (manual) | auth pii | approve | auth-scope PASS |
| G4 (7-8 gateway) | tasks.md 7-8 | approved | 1 | 99050f41 | PASS | auth | approve | auth-scope PASS |
## Gates
## Escalations
- FEATURES flip planned→automated (+covered_by auth/session_refresh.feature::<scenario>) to do after green (e2e-author hard rule).
- Tooling: red_first.sh regenerates proto via remote BSR plugins; BSR rate-limited 2026-10-14, so G2/G3 red-first was reproduced manually (temp worktrees at base/head, copied team-identity/generated, head test files on base). team-identity has no buf.gen.local.yaml (team-gateway does) — adding one would let red_first use the installed local plugins.
## Gates (final)  e2e=16/16 green (stack q1, identity+gateway rebuilt from slice) features-check=pass spec_sync --strict=pass (17 e2e, 27 unit-verified) feature-reviewer=pass 17/17 decide=ENGINE_DOWN (LLM fallback) privacy=needs-human
- privacy NEEDS_HUMAN: (1) lawful basis "contract" for 30-day refresh cookie (carried from contract slice); (2) expired-but-unrevoked sessions keep refresh hashes + device/IP indefinitely — proposal says "until refresh_expires_at": implement cleanup job or amend proposal wording.
- auth note for wave 2: gateway admit() 401s a stale bearer on public routes → frontend must call RefreshSession WITHOUT Authorization (spec already says so).
