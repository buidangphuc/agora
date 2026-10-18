# [W1-T1] audit-query-filters

## Role
feature-run worker (change `openspec/changes/archive/2026-10-18-audit-query-filters`)

## Objective
Implement OpenSpec change `audit-query-filters` test-first per its tasks.md (scenario ids there); context in plans/q3-audit-log/00-spec.md and the change's design.md Clarifications.
- Includes the team-gateway forwarder test (gateway only regenerates its gitignored stubs) and the shared e2e helper `platform-e2e/tests/e2e/support/audit_support.py`, which this slice alone writes; every wave-2 slice imports it read-only.

## Write-set (EXCLUSIVE)
- team-audit/migrations/002_audit_action_created_index.up.sql
- team-audit/migrations/002_audit_action_created_index.down.sql
- team-audit/internal/repository/pgtest_audit_test.go
- team-audit/internal/repository/audit.go
- team-audit/internal/repository/audit_filter_test.go
- team-audit/internal/service/audit.go
- team-audit/internal/service/audit_test.go
- team-audit/internal/handler/audit.go
- team-audit/internal/handler/audit_test.go
- team-audit/README.md
- team-audit/FEATURES.yaml
- team-gateway/internal/edge/audit_test.go
- team-gateway/internal/edge/policy_test.go
- platform-e2e/tests/e2e/support/audit_support.py
- platform-e2e/tests/e2e/features/security/audit_query_filters.feature
- platform-e2e/tests/e2e/step_definitions/test_audit_query_filters.py

## Read-only dependencies
- openspec/changes/archive/2026-10-18-audit-query-filters/** (frozen by LOCK.md digest)
- team-audit/proto/platform/audit/v1/audit.proto (vendored by q3-audit-log-contract; regenerate gitignored `generated/` with `make proto`, never hand-edit)
- team-gateway/proto/platform/audit/v1/audit.proto (vendored by q3-audit-log-contract; regenerate gitignored `generated/` with `make proto`, never hand-edit)
- plans/q3-audit-log/00-spec.md, plans/q3-audit-log/slices.yaml, plans/q3-audit-log/LOCK.md

## Verify
(cd team-audit && make proto && go test ./...); (cd team-gateway && make proto && go test ./internal/edge/...); (cd platform-e2e && HEADLESS=true /Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python -m pytest -q tests/e2e/features/security/audit_query_filters.feature); platform-e2e/.venv/bin/python platform-e2e/scripts/spec_sync.py audit-query-filters --strict
