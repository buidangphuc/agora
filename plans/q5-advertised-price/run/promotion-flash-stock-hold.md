# feature-run promotion-flash-stock-hold
initiative: q5-advertised-price | base: ed6ee6fb | worktree: .claude/worktrees/promotion-flash-stock-hold | branch: asdlc/promotion-flash-stock-hold | started: 2026-10-16
## Preflight   validate=p spec_lint=p
## E2E         author=done run=red as expected (Unimplemented); round 2: replay-id defect in steps fixed, scenarios renamed to echo spec titles
## Tasks
| task | scenarios | status | rounds | T0 | red_first | routed | task-reviewer | specialists |
|---|---|---|---|---|---|---|---|---|
| 1-6 | all code-track (33 unit + wiring) | approved | 1 | 76106bb1 | PASS | (auth by spec) | approve | auth-scope PASS |
## Gates       e2e=14/14 green (q2 stack, team-promotion:local, migration 0004) features-check=p spec_sync --strict=p (14 e2e + 33 unit) feature-reviewer=pass 14/14 decide=ENGINE_DOWN (LLM fallback); make check + Postgres tests (promotion_test) green
## Escalations none. Drift: spec VERIFIED BY lines cite old e2e scenario names (renamed); campaigns seeded by seller (admin CreateCampaign needs team-audit).
merged: b4642001
