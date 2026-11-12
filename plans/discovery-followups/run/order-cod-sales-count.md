# feature-run order-cod-sales-count
initiative: discovery-followups | base: bc90c721 | worktree: .claude/worktrees/order-cod-sales-count | branch: asdlc/order-cod-sales-count | started: 2026-11-12
## Preflight   validate=p spec_lint=p spec_sync=planned (initiative-plan)
## E2E         author=done run=not run (stack down; live run at converge) commit=c8efe84e 16 entries
## Tasks
| task | scenarios | status | rounds | T0 | red_first | routed | task-reviewer | specialists |
|---|---|---|---|---|---|---|---|---|
| T1 (1-6) | all store+migration+rpc | approved | 1 (test-defect lastMigration=23) | c8efe84e | pass (rpc guard exempt) | – | approve (6 NIT) | – |
## Gates       e2e=– features-check=– spec_sync --strict=– feature-reviewer=– decide=ENGINE_DOWN (LLM fallback) privacy=–
## Escalations
