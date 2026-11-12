# feature-run analytics-listing-deleted-status
initiative: discovery-followups | base: bc90c721 | worktree: .claude/worktrees/analytics-listing-deleted-status | branch: asdlc/analytics-listing-deleted-status | started: 2026-11-12
## Preflight   validate=p spec_lint=p spec_sync=planned (initiative-plan)
## E2E         author=done run=not run (stack down; live run at converge) commit=3ce4621d 3 e2e + unit entries
## Tasks
| task | scenarios | status | rounds | T0 | red_first | routed | task-reviewer | specialists |
|---|---|---|---|---|---|---|---|---|
| T1 (1.x,2.x,3.x) | all | approved | 0 | 3ce4621d | pass (py needs REDFIRST_PYTHON venv + PYSPARK_PYTHON) | – | approve (3 NIT, 2 are comment-only and skipped per no-comments rule) | – |
## Gates       e2e=– features-check=– spec_sync --strict=– feature-reviewer=– decide=ENGINE_DOWN (LLM fallback) privacy=–
## Escalations
