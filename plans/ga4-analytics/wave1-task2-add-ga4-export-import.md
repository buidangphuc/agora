# [W1-T2] add-ga4-export-import

## Role
feature-run worker (change `openspec/changes/add-ga4-export-import`)

## Objective
Implement OpenSpec change `add-ga4-export-import` test-first per its tasks.md (scenario ids there); context in plans/ga4-analytics/00-spec.md.

## Write-set (EXCLUSIVE)
- team-analytics/internal/warehouse/** (tasks.md wh-ga4 write-set)
- team-analytics/internal/ga4import/**
- team-analytics/internal/consumer/tracking.go
- team-analytics/internal/consumer/tracking_test.go
- team-analytics/cmd/ga4import/**
- team-analytics/cmd/consumer/** (reconcile wiring, /metrics, in-process GA4 import + internal trigger)
- team-analytics/Dockerfile (build /ga4import next to /consumer; e2e runs it in the analytics image)
- team-analytics/internal/config/config.go
- team-analytics/internal/config/config_test.go
- team-analytics/.env.example (kept in sync with config by TestEnvExampleInSync)
- team-analytics/internal/query/**
- team-analytics/internal/grpcserver/**
- team-analytics/internal/observability/**
- team-analytics/internal/export/export.go
- team-analytics/internal/export/export_test.go
- team-analytics/FEATURES.yaml
- platform-core/FEATURES.yaml (flip W0 tracking-consent.channel-funnel-* to automated)
- platform-gitops/platform/team-analytics/** (tasks.md gitops-ga4 / ga4import-in-consumer write-set)
- platform-gitops/platform/monitoring/alerts.yaml
- platform-gitops/platform/monitoring/prometheus.yaml (scrape team-analytics :9109)
- platform-e2e/scripts/tests/test_gitops_ga4.py (gitops render + promtool tests)
- platform-gitops/envs/** (concrete env files TBD)
- team-gateway/internal/edge/policy.go
- team-gateway/internal/edge/policy_test.go
- team-gateway/internal/edge/policy_edge_test.go
- team-gateway/internal/edge/analytics.go (forward GetChannelFunnel; edge answered 501)
- team-gateway/internal/edge/analytics_test.go
- platform-e2e/tests/e2e/features/tracking/ga4_export_import.feature
- platform-e2e/tests/e2e/step_definitions/ga4_export_import_steps.py
- platform-e2e/tests/e2e/step_definitions/test_ga4_export_import.py

- openspec/changes/add-ga4-export-import/specs/ga4-export-import/spec.md (spec-clarify only, human-approved re-lock)
- openspec/changes/add-ga4-export-import/tasks.md (spec-clarify only)
- openspec/changes/add-ga4-export-import/design.md (spec-clarify only)
- openspec/changes/add-ga4-export-import/specs/tracking-data-quality/spec.md (spec-clarify VERIFIED BY only)

## Read-only dependencies
- openspec/changes/add-ga4-export-import/** (frozen by LOCK.md digest)
- plans/ga4-analytics/00-spec.md, plans/ga4-analytics/slices.yaml

## Verify
(cd team-analytics && buf generate/make proto && go vet ./... && go test ./...); (cd team-gateway && buf generate --template buf.gen.local.yaml && go test ./internal/edge/...); platform-e2e/.venv/bin/python platform-e2e/scripts/spec_sync.py add-ga4-export-import --strict; (cd platform-e2e && HEADLESS=true .venv/bin/python -m pytest -q tests/e2e/step_definitions/test_ga4_export_import.py)
