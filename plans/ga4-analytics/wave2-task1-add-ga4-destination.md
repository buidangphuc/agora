# [W2-T1] add-ga4-destination

## Role
feature-run worker (change `openspec/changes/add-ga4-destination`)

## Objective
Implement OpenSpec change `add-ga4-destination` test-first per its tasks.md (scenario ids there); context in plans/ga4-analytics/00-spec.md.

## Write-set (EXCLUSIVE)
- team-frontend/analytics/gtm-container.json
- team-frontend/src/lib/analytics/ga4/userId.ts
- team-frontend/src/lib/analytics/ga4/userId.test.ts
- team-frontend/src/app/api/analytics/identity/route.ts
- team-frontend/src/app/api/analytics/identity/route.test.ts
- team-frontend/src/lib/analytics/destinations/gtm.ts
- team-frontend/src/lib/analytics/destinations/gtm.test.ts
- team-frontend/src/features/tracking/AnalyticsProvider.tsx
- team-frontend/src/features/tracking/AnalyticsProvider.test.tsx
- team-frontend/next.config.mjs
- team-frontend/Dockerfile
- team-frontend/src/lib/analytics/map.ts
- team-frontend/src/lib/analytics/queue.ts
- team-frontend/src/lib/analytics/schema.ts
- team-frontend/src/lib/analytics/dispatcher.ts
- team-frontend/src/lib/analytics/dispatcher.test.ts
- team-frontend/src/lib/analytics/queue.test.ts
- team-frontend/src/lib/analytics/gtm-container.test.ts
- team-frontend/src/lib/analytics/map.test.ts
- team-frontend/src/next-config.test.ts
- team-gateway/internal/edge/session_principal.go
- team-gateway/internal/edge/session_principal_test.go
- team-gateway/internal/edge/bearer_test.go
- team-gateway/internal/edge/server.go
- team-gateway/FEATURES.yaml
- openspec/changes/add-ga4-destination/**
- team-frontend/README.md
- team-frontend/FEATURES.yaml (owner assumed; tasks.md names none)
- platform-e2e/tests/e2e/features/tracking/ga4_destination.feature
- platform-e2e/tests/e2e/step_definitions/ga4_destination_steps.py
- platform-e2e/tests/e2e/step_definitions/test_ga4_destination.py

## Read-only dependencies
- openspec/changes/add-ga4-destination/** (frozen by LOCK.md digest)
- plans/ga4-analytics/00-spec.md, plans/ga4-analytics/slices.yaml

## Verify
(cd team-frontend && npx vitest run src/lib/analytics src/app/api/analytics src/features/tracking && npm run typecheck && npx prettier --check <changed files>); platform-e2e/.venv/bin/python platform-e2e/scripts/spec_sync.py add-ga4-destination --strict; (cd platform-e2e && HEADLESS=true .venv/bin/python -m pytest -q tests/e2e/step_definitions/test_ga4_destination.py)
