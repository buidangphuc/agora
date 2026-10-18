# [W1-T1] analytics-review-hidden

## Role
feature-run worker (change `openspec/changes/analytics-review-hidden`)

## Objective
Implement OpenSpec change `analytics-review-hidden` test-first per its tasks.md (scenario ids there); context in plans/privacy-audit-hardening/00-spec.md and LOCK.md "Open issues" decisions.
- team-analytics maps engagement.events ReviewHidden to a review_hidden engagement fact (hidden_at, else envelope time), adds a nullable review_id column (idempotent migration) set on review_created and review_hidden; Parquet export carries hidden rows.
- depends_on: pah-contract

## Write-set (EXCLUSIVE)
- team-analytics/internal/consumer/engagement.go
- team-analytics/internal/consumer/engagement_test.go
- team-analytics/internal/warehouse/warehouse.go
- team-analytics/internal/warehouse/warehouse_test.go
- team-analytics/internal/warehouse/duckdb/duckdb.go
- team-analytics/internal/warehouse/duckdb/duckdb_test.go
- team-analytics/README.md
- team-analytics/FEATURES.yaml
- platform-e2e/tests/e2e/features/analytics/review_hidden.feature
- platform-e2e/tests/e2e/step_definitions/test_analytics_review_hidden.py
- openspec/changes/analytics-review-hidden/**

## Read-only dependencies
- openspec/changes/analytics-review-hidden/** is frozen by the LOCK.md digest (only task ticks change)
- team-analytics/proto/platform/engagement/v1/events.proto (copied by pah-contract)
- platform-e2e/tests/e2e/step_definitions/test_efe_warehouse.py, efe_steps.py (envelope/produce and warehouse-rows helpers; reuse, do not edit)
- plans/privacy-audit-hardening/00-spec.md, slices.yaml, LOCK.md

## Verify
(cd team-analytics && make proto && go vet ./... && go test ./... -race); (cd platform-e2e && HEADLESS=true $PY -m pytest -q tests/e2e/step_definitions/test_analytics_review_hidden.py); $PY platform-e2e/scripts/spec_sync.py analytics-review-hidden --strict
