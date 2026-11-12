# feature-run discovery-followups-contract
initiative: discovery-followups | base: 677be27b | worktree: .claude/worktrees/discovery-followups-contract | branch: asdlc/discovery-followups-contract | started: 2026-11-12
## Preflight   validate=p spec_lint=p spec_sync=planned (initiative-plan)
## E2E         author=done run=n/a (FEATURES not-testable, 18 entries) files=1
## Tasks
| task | scenarios | status | rounds | T0 | red_first | routed | task-reviewer | specialists |
|---|---|---|---|---|---|---|---|---|
| T1 (1.1-1.5) | all | approved | 1 (test-defect: moderation pin, buf enum grep; FE stubs/tgz added) | 677be27b | pass (3 files) | auth payment pii proto | approve (3 NIT) | contract-boundary PASS (3 NIT), auth-scope PASS (2 NIT) |
## Gates       e2e=n/a (contract) features-check=pass spec_sync --strict=pass (18 unit-verified) make-check=pass proto:check=pass vendor:check=pass docs-sync=team-search README row feature-reviewer=– decide=ENGINE_DOWN (LLM fallback) privacy=needs-human (stale-copy parity brings Storefront/ShopSummary/moderation note/seller_id defs into chat, identity, payment, engagement, notification, promotion; defs only, nothing stored or logged; DPO to confirm covered by earlier classification)
## Escalations
