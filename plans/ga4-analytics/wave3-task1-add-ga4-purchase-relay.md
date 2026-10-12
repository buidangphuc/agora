# [W3-T1] add-ga4-purchase-relay

## Role
feature-run worker (change `openspec/changes/add-ga4-purchase-relay`)

## Objective
Implement OpenSpec change `add-ga4-purchase-relay` test-first per its tasks.md (scenario ids there); context in plans/ga4-analytics/00-spec.md.

## Write-set (EXCLUSIVE)
- team-analytics/internal/config/config.go
- team-analytics/internal/config/config_test.go
- team-analytics/.env.example
- team-analytics/internal/ga4relay/mapping.go
- team-analytics/internal/ga4relay/mapping_test.go
- team-analytics/internal/ga4relay/client.go
- team-analytics/internal/ga4relay/client_test.go
- team-analytics/internal/ga4relay/relay.go
- team-analytics/internal/ga4relay/relay_test.go
- team-analytics/internal/ga4relay/**
- team-analytics/cmd/ga4-relay/**
- team-analytics/Dockerfile
- team-analytics/Makefile
- team-analytics/go.mod
- team-analytics/go.sum
- team-analytics/FEATURES.yaml
- team-frontend/src/lib/analytics/map.ts
- team-frontend/src/lib/analytics/map.test.ts
- team-frontend/src/lib/track.ts
- team-frontend/src/lib/track.test.ts
- team-frontend/src/lib/analytics/dispatcher.ts
- platform-gitops/platform/team-analytics/ga4-relay.yaml
- platform-gitops/platform/team-analytics/kustomization.yaml
- platform-gitops/platform/infra/redpanda.yaml
- platform-gitops/tests/test_ga4_relay_manifests.py
- platform-e2e/tests/e2e/features/tracking/ga4_purchase_relay.feature
- platform-e2e/tests/e2e/step_definitions/ga4_purchase_relay_steps.py
- platform-e2e/tests/e2e/step_definitions/test_ga4_purchase_relay.py
- platform-e2e/tests/e2e/features/tracking/ga4_purchase_browser.feature
- platform-e2e/tests/e2e/step_definitions/ga4_purchase_browser_steps.py
- platform-e2e/tests/e2e/step_definitions/test_ga4_purchase_browser.py
- platform-e2e/tests/e2e/support/ga4_relay_support.py
- platform-e2e/tests/e2e/features/journeys/buyer_full_funnel_journey.feature
- platform-e2e/tests/e2e/features/frontend/ui_checkout_order.feature
- platform-e2e/tests/e2e/step_definitions/**
- platform-e2e/tests/e2e/features/buyer/cart_checkout.feature
- team-frontend/FEATURES.yaml
- openspec/changes/add-ga4-purchase-relay/**

## Read-only dependencies
- openspec/changes/add-ga4-purchase-relay/** (frozen by LOCK.md digest)
- plans/ga4-analytics/00-spec.md, plans/ga4-analytics/slices.yaml

## Verify
(cd team-analytics && go vet ./... && go test ./...); (cd team-frontend && npx vitest run src/lib/analytics/map.test.ts && npm run typecheck && npx prettier --check src/lib/analytics/map.ts src/lib/analytics/map.test.ts); platform-e2e/.venv/bin/python platform-e2e/scripts/spec_sync.py add-ga4-purchase-relay --strict; (cd platform-e2e && HEADLESS=true .venv/bin/python -m pytest -q tests/e2e/step_definitions/test_ga4_purchase_relay.py)
