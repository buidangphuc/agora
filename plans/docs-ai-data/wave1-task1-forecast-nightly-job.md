# [W1-T1] forecast-nightly-job

## Role
feature-run worker (change `openspec/changes/forecast-nightly-job`)

## Objective
Implement OpenSpec change `forecast-nightly-job` test-first per its tasks.md; context in plans/docs-ai-data/00-spec.md.
- platform-forecast becomes a nightly job: DuckDB reader over `order_facts.parquet`, global quantile LightGBM vs seasonal naive by pooled WAPE, per-listing `SETEX fc:v1:...` in Redis DB 3, `fc:v1:meta` last; Dockerfile (non-root), compose `forecast-job` (profile `forecast`), gitops CronJob `45 2 * * *`, CI job, ADR-0013 Accepted.
- depends_on: none
- base: `asdlc/docs-ai-data` after wave 0
- hot file: docker-compose.services.yaml (shared with recs-experiment-arms W2, analytics-forecast-serving W3, ai-assistant-grounding W4)

## Write-set (EXCLUSIVE)
- platform-forecast/**
- platform-gitops/platform/forecast/**
- platform-gitops/argocd/apps/forecast.yaml
- docker-compose.services.yaml
- .github/workflows/services-ci.yml
- AGENTS.md
- platform-core/docs/ADR/0013-seller-demand-forecasting.md
- platform-e2e/tests/e2e/features/forecast/**
- platform-e2e/tests/e2e/step_definitions/test_nightly_forecast.py
- platform-e2e/tests/e2e/step_definitions/nightly_forecast_steps.py
- platform-e2e/tests/e2e/flows/forecast_flow.py
Widened (P1):
  - docs/agents/project.md — docs-sync: platform-forecast unit loop and CI job rows
  - platform-core/README.md — docs-sync: ADR-0013 status Proposed to Accepted in the ADR table
  - platform-gitops/README.md — docs-sync: forecast raw-manifest app and platform/forecast kustomization
  - platform-e2e/tests/e2e/flows/fnj_job_driver.py — e2e support (flow driver) of this change under a different name
  - platform-e2e/tests/e2e/flows/fnj_job_flow.py — e2e support (flow) of this change under a different name
  - platform-e2e/tests/e2e/step_definitions/fnj_steps.py — e2e step file of this change under a different name
  - platform-e2e/tests/e2e/step_definitions/test_fnj_nightly_forecast.py — e2e scenario binding of this change under a different name

## Verify
- `cd platform-forecast && uv run pytest -q`
- `docker compose --profile forecast run --rm forecast-job` against the local stack (exit 0, `fc:v1:meta` present in Redis DB 3)
- `make -C platform-e2e features-check`
- e2e: forecast/nightly_forecast.feature
