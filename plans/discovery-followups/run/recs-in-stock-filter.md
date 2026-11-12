# feature-run recs-in-stock-filter
initiative: discovery-followups | base: 10d70496 | worktree: .claude/worktrees/recs-in-stock-filter | branch: asdlc/recs-in-stock-filter | started: 2026-11-12
## Preflight   validate=p spec_lint=p
## E2E         author=done run=not run (stack down; live at converge) commit=f2cb8731
## Tasks
| task | scenarios | status | rounds | T0 | red_first | routed | task-reviewer | specialists |
|---|---|---|---|---|---|---|---|---|
| T1 (1.1-1.6) | all | approved | 1 (6 fixture defects) | f2cb8731 | pass (5 exempt fixture/guard) | – | approve (3 NIT) | auth-scope PASS (2 NIT: scopes configurable; insecure channel relies on network isolation) |
## Gates       e2e=– features-check=– feature-reviewer=– decide=ENGINE_DOWN (LLM fallback)
## Escalations
- Settings: RECS_SEARCH_ADDR, RECS_SEARCH_PRINCIPAL_ID, RECS_SEARCH_PRINCIPAL_SCOPES; empty result fallback flag False on ladder path, True on popular path (left to implementer per LOCK).
