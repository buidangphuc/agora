# [W4-T1] analytics-experiment-reports

## Role
feature-run worker (change `openspec/changes/analytics-experiment-reports`)

## Objective
Implement OpenSpec change `analytics-experiment-reports` test-first per its tasks.md; context in plans/docs-ai-data/00-spec.md.
- team-analytics: `GetRetentionCohorts`, `GetExperimentReadout` (Wilson 95%), `GetAiSurfaceReport` over DuckDB, admin-only, weeks 1..26; tracking maps `EVENT_TYPE_EXPERIMENT_EXPOSURE` -> `experiment_exposure`. team-gateway: admin-gated procedures, forwarders, beacon allowlist exactly `flag`/`variant`.
- depends_on: docs-ai-data-contract, analytics-forecast-serving (merged)
- base: `asdlc/docs-ai-data` after wave 3
- MINOR to fold in: young-cohort scenario "with `from` set to the Monday of that cohort week".

## Write-set (EXCLUSIVE)
- team-analytics/internal/query/**
- team-analytics/internal/rpc/**
- team-analytics/internal/events/**
- team-analytics/internal/config/**
- team-analytics/README.md
- team-analytics/FEATURES.yaml
- team-gateway/internal/edge/**
- team-gateway/README.md
- team-gateway/FEATURES.yaml
- platform-e2e/tests/e2e/features/analytics/retention_cohorts.feature
- platform-e2e/tests/e2e/features/analytics/experiment_readout.feature
- platform-e2e/tests/e2e/features/analytics/ai_surface_report.feature
- platform-e2e/tests/e2e/features/analytics/experiment_reports_access.feature
- platform-e2e/tests/e2e/features/tracking/experiment_exposure.feature
- platform-e2e/tests/e2e/step_definitions/test_retention_cohorts.py
- platform-e2e/tests/e2e/step_definitions/test_experiment_readout.py
- platform-e2e/tests/e2e/step_definitions/test_ai_surface_report.py
- platform-e2e/tests/e2e/step_definitions/test_experiment_reports_access.py
- platform-e2e/tests/e2e/step_definitions/test_experiment_exposure.py
- platform-e2e/tests/e2e/step_definitions/experiment_reports_steps.py
- platform-e2e/tests/e2e/step_definitions/tracking_steps.py
- widened (P1):
  - openspec/changes/analytics-experiment-reports/design.md — file of this change's OpenSpec folder: Clarifications entry recording the user's D4 answer (no requirement or scenario wording change)
  - openspec/changes/analytics-experiment-reports/specs/analytics-experiment-reports/spec.md — file of this change's OpenSpec folder: VERIFIED BY citations corrected to the real e2e scenario and Go test names
  - platform-e2e/tests/e2e/step_definitions/test_analytics_ai_surface_report.py — e2e step file of this change (analytics/ai_surface_report.feature), named differently from the packet
  - platform-e2e/tests/e2e/step_definitions/test_analytics_experiment_readout.py — e2e step file of this change (analytics/experiment_readout.feature), named differently from the packet
  - platform-e2e/tests/e2e/step_definitions/test_analytics_experiment_reports_access.py — e2e step file of this change (analytics/experiment_reports_access.feature), named differently from the packet
  - platform-e2e/tests/e2e/step_definitions/test_analytics_retention_cohorts.py — e2e step file of this change (analytics/retention_cohorts.feature), named differently from the packet
  - platform-e2e/tests/e2e/step_definitions/test_tracking_experiment_exposure.py — e2e step file of this change (tracking/experiment_exposure.feature), named differently from the packet
  - platform-e2e/tests/e2e/support/experiment_reports_support.py — e2e support file of this change

## Verify
- `cd team-analytics && DOCKER_HOST=unix:///Users/phuc.buidang/.orbstack/run/docker.sock GO_SDK_REQUIRE_DOCKER=1 go test ./...`
- `cd team-gateway && buf generate --template buf.gen.local.yaml && go test ./...`
- `openspec validate analytics-experiment-reports --strict`
- `make -C platform-e2e features-check`
- e2e: the five feature files above (redpanda, team-analytics, team-gateway, team-identity + buyer deps only)
