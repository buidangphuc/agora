# feature-run q1-session-refresh-contract
initiative: q1-session-continuity | base: 79673995 | worktree: .claude/worktrees/q1-session-refresh-contract | branch: asdlc/q1-session-refresh-contract
## Preflight   validate=p spec_lint=p spec_sync=8 uncovered
## E2E         author=done (FEATURES not-testable entries only; e2e track none) run=n/a files=1
## Tasks
| task | scenarios | status | rounds | T0 | red_first | routed | task-reviewer | specialists |
|---|---|---|---|---|---|---|---|---|
| T1 (1.1-2.2) | all 9 | approved | 2 | 79673995 | PASS | auth pii proto | approve | contract-boundary PASS; auth-scope safe (0 BLOCKING) |
## Gates       e2e=n/a features-check=pass spec_sync --strict=pass feature-reviewer=pass (9/9) decide=ENGINE_DOWN (LLM fallback) privacy=needs-human (lawful basis "contract" for 30-day refresh cookie; DPO)
## Escalations
- none. Notes: task 2.2 per-language generated grep not evidenced (gen/ needs network buf plugins); vendored gateway/frontend copies also pick up pre-existing PublicProfileService (stale copies); go 1.22→1.23 in proto module.
## Merged into asdlc/q1-session-continuity at df46fecb
