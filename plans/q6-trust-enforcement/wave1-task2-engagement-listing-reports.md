# [W1-T2] engagement-listing-reports

## Role
feature-run worker (change `openspec/changes/engagement-listing-reports`)

## Objective
Implement OpenSpec change `engagement-listing-reports` test-first per its tasks.md (scenario ids there); context in plans/q6-trust-enforcement/00-spec.md and the change's design.md Clarifications.
- team-engagement listing reports: CreateReport (dedupe open report, cap 20, no self-report), admin ListReports/ResolveReport (audit-first report.resolve), 2-year retention sweep with pseudonymisation; gateway forwarders and admin policy.
- depends_on: q6-trust-contract

## Write-set (EXCLUSIVE)
- team-engagement/migrations/0012_reports.up.sql
- team-engagement/migrations/0012_reports.down.sql
- team-engagement/internal/repository/report.go
- team-engagement/internal/repository/report_test.go
- team-engagement/internal/repository/pgtest_report_test.go
- team-engagement/internal/service/report.go
- team-engagement/internal/service/report_test.go
- team-engagement/internal/upstream/audit.go
- team-engagement/internal/upstream/audit_test.go
- team-engagement/internal/handler/reports.go
- team-engagement/internal/handler/reports_test.go
- team-engagement/internal/handler/engagement.go
- team-engagement/internal/config/config.go
- team-engagement/internal/config/config_test.go
- team-engagement/internal/bootstrap/resources.go
- team-engagement/internal/bootstrap/lifecycle.go
- team-engagement/cmd/**
- team-engagement/proto/**
- team-engagement/generated/**
- team-engagement/.env.example
- team-engagement/README.md
- team-engagement/FEATURES.yaml
- team-gateway/internal/edge/engagement.go
- team-gateway/internal/edge/policy.go
- team-gateway/internal/edge/policy_test.go
- team-gateway/internal/edge/policy_edge_test.go
- team-gateway/internal/edge/unrouted_test.go
- team-gateway/internal/edge/policy_edge_reports_test.go
- team-gateway/generated/**
- team-gateway/FEATURES.yaml
- platform-e2e/tests/e2e/support/report_support.py
- platform-e2e/src/api/services/engagement_service.py
- platform-e2e/tests/e2e/features/engagement/listing_reports.feature
- platform-e2e/tests/e2e/step_definitions/test_listing_reports.py
- openspec/changes/engagement-listing-reports/**

## Read-only dependencies
- openspec/changes/engagement-listing-reports/** is frozen by the LOCK.md digest (only task ticks change)
- platform-e2e/tests/e2e/support/audit_support.py (reuse, do not edit)
- team-engagement hidereview_roundtrip_test.go (pattern)
- plans/q6-trust-enforcement/00-spec.md, slices.yaml, LOCK.md

## Verify
(cd team-engagement && make proto && go vet ./... && go test ./... -race); (cd team-gateway && go vet ./... && go test ./internal/edge/... -race); (cd platform-e2e && HEADLESS=true $PY -m pytest -q tests/e2e/step_definitions/test_listing_reports.py -m 'not destructive') then the @destructive audit-down scenario alone; $PY platform-e2e/scripts/spec_sync.py engagement-listing-reports --strict
