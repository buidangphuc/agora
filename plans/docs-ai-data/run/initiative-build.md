# initiative-build — docs-ai-data
root: /Users/phuc.buidang/Documents/agora-docs-ai-data (feat/docs-ai-data)
integration: .claude/worktrees/docs-ai-data (asdlc/docs-ai-data)
base: origin/main @ 06c44c17
wip_limit: 3
| wave | slices | depends_on | base | status |
|---|---|---|---|---|
| 0 | docs-ai-data-contract | – | asdlc/docs-ai-data | pending |
| 1 | forecast-nightly-job | – | asdlc/docs-ai-data after W0 | pending |
| 2 | recs-experiment-arms | – | asdlc/docs-ai-data after W1 | pending |
| 3 | analytics-forecast-serving | contract, forecast-nightly-job | asdlc/docs-ai-data after W2 | pending |
| 4 | analytics-experiment-reports, ai-assistant-grounding | contract + analytics-forecast-serving; contract + recs-experiment-arms | asdlc/docs-ai-data after W3 | pending |
| 5 | ai-copilot-grounding, admin-insights-panels | contract + ai-assistant-grounding; analytics-experiment-reports | asdlc/docs-ai-data after W4 | pending |
| 6 | seller-forecast-card, buyer-ai-surfaces-ui | analytics-forecast-serving + ai-copilot-grounding; ai-assistant-grounding + ai-copilot-grounding + analytics-experiment-reports | asdlc/docs-ai-data after W5 | pending |
Per-slice state: plans/docs-ai-data/run/<slice>.md

## Wave rationale
- Contract alone in wave 0.
- docker-compose.services.yaml is written by forecast-nightly-job, recs-experiment-arms, analytics-forecast-serving and ai-assistant-grounding, so each sits in its own wave (W1, W2, W3, W4).
- team-ai hot files (core/config, bootstrap/services.py, servicers/ai.py, FEATURES.yaml): recs W2, assistant W4, copilot W5.
- team-analytics hot files (rpc/handler.go, query/{repository,duckdb}.go, FEATURES.yaml): forecast-serving W3, experiment-reports W4, seller-forecast-card (FEATURES.yaml only) W6.
- Digests checked against LOCK.md on 2026-11-13: all 10 match.

## Carry-forward
- Env: PY=/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python; symlink the venv into the worktree's platform-e2e/ (never commit it).
- Go DB tests: DOCKER_HOST=unix:///Users/phuc.buidang/.orbstack/run/docker.sock GO_SDK_REQUIRE_DOCKER=1.
- Generated stubs are gitignored: team-gateway `buf generate --template buf.gen.local.yaml`; FE apps `npm ci && npm run proto`.
- Open issue 19 (LOCK.md): recs-experiment-arms writes RECS_FLIPT_URL and RECS_EXPERIMENT_SALT into docker-compose.services.yaml; its packet owns that file in W2.
