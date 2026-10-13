# feature-run add-model-registry
initiative: training-platform | base: 84d59c0c | worktree: .claude/worktrees/add-model-registry | branch: asdlc/add-model-registry | started: 2026-10-13
## Preflight   validate=p spec_lint=p spec_sync=70 uncovered
## E2E         author=done run=red as expected files=4 (checkpoint e2e red)
## Tasks
| task | scenarios | status | rounds | T0 | red_first | routed | task-reviewer | specialists |
|---|---|---|---|---|---|---|---|---|
| 1 ADR | 1.x | approved | 1 | a81c3180 | PASS | - | approve | - |
| 2 compose infra | 2.x | approved | 1 | 4589eec7 | PASS | - | approve | - |
| 3 library | 3.x | approved | 1 (+hardening) | ff726ccd | PASS (lib venv) | - | approve | - |
| 4 gitops | 4.x | approved | 1 (+cluster fixes) | fcd5ae2a | PASS | - | approve | - |
| 5 CI | 5.x | todo | 0 | | | | | |
## Gates
## Escalations
- build note (task 3): MLflow 3.2 rejects `@` in metric names; the library encodes `@`→`_at_` on write and decodes on read, transparent to callers and policies (no requirement change).
- note: red_first for py-modelregistry needs REDFIRST_PYTHON with mlflow-skinny; CI tdd job uses platform-e2e venv → check at converge.
- deferred minors (task 4): vault kv put guard, pg_isready -U, mlflow image in push script + sha tag, prometheus/finetune labels.
