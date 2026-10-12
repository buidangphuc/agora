# [W1-T1] add-consent-mode

## Role
feature-run worker (change `openspec/changes/add-consent-mode`)

## Objective
Implement OpenSpec change `add-consent-mode` test-first per its tasks.md (scenario ids there); context in plans/ga4-analytics/00-spec.md.

## Write-set (EXCLUSIVE)
- team-frontend/src/lib/consent/consent.ts
- team-frontend/src/lib/consent/consent.test.ts
- team-frontend/src/features/consent/ConsentBanner.tsx
- team-frontend/src/features/consent/ConsentBanner.test.tsx
- team-frontend/src/app/layout.tsx
- team-frontend/src/components/shell/ConsumerShell.tsx (footer 'Cookie settings' link)
- team-frontend/src/lib/analytics/destinations/gtm.ts
- team-frontend/src/lib/analytics/destinations/gtm.test.ts
- team-frontend/src/features/tracking/AnalyticsProvider.tsx
- team-frontend/src/features/tracking/AnalyticsProvider.test.tsx
- team-frontend/src/lib/analytics/queue.test.ts
- team-frontend/src/lib/analytics/dispatcher.test.ts
- team-frontend/src/lib/analytics/dataLayer.ts
- team-frontend/src/lib/analytics/dataLayer.test.ts
- team-frontend/src/lib/analytics/dataLayer.privacy.test.ts (seed a=1 consent in setup)
- team-frontend/src/test/listBeacons.ts (test helper seeds a=1 consent)
- team-frontend/src/lib/track.test.ts (seed a=1 consent)
- team-frontend/src/features/tracking/listImpressions.test.tsx (seed a=1 consent)
- team-frontend/FEATURES.yaml
- team-gateway/internal/edge/collector.go
- team-gateway/internal/edge/consent_cookie.go
- team-gateway/internal/edge/consent_cookie_test.go
- team-gateway/internal/edge/collector_test.go
- team-gateway/internal/edge/collector_boundary_test.go
- team-gateway/FEATURES.yaml
- platform-e2e/tests/e2e/features/tracking/consent.feature
- platform-e2e/tests/e2e/step_definitions/consent_steps.py
- platform-e2e/tests/e2e/step_definitions/test_consent.py
- platform-e2e/tests/e2e/features/tracking/consent_cls.feature
- platform-e2e/tests/e2e/features/tracking/ga4_datalayer.feature (pre-seed consent cookie; GTM now gated)
- platform-e2e/tests/e2e/step_definitions/ga4_datalayer_steps.py
- platform-e2e/tests/e2e/support/world.py (pre-seed reject-all consent; @freshVisitor opts out)
- platform-e2e/pyproject.toml (register @freshVisitor marker)
- platform-e2e/tests/conftest.py (world fixture calls setup_web_context)

- openspec/changes/add-consent-mode/specs/tracking/spec.md (spec-clarify only, human-approved re-lock)
- openspec/changes/add-consent-mode/tasks.md (spec-clarify only)
- openspec/changes/add-consent-mode/specs/consent/spec.md (spec-clarify VERIFIED BY only)

## Read-only dependencies
- openspec/changes/add-consent-mode/** (frozen by LOCK.md digest)
- plans/ga4-analytics/00-spec.md, plans/ga4-analytics/slices.yaml

## Verify
(cd team-frontend && npx vitest run src/lib/consent src/features/consent src/lib/analytics src/features/tracking && npm run typecheck && npx prettier --check <changed files>); (cd team-gateway && buf generate --template buf.gen.local.yaml && go vet ./... && go test ./internal/edge/...); platform-e2e/.venv/bin/python platform-e2e/scripts/spec_sync.py add-consent-mode --strict; (cd platform-e2e && HEADLESS=true .venv/bin/python -m pytest -q tests/e2e/step_definitions/test_consent.py)
