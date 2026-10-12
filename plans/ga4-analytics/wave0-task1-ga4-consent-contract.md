# [W0-T1] ga4-consent-contract

## Role
feature-run worker (change `openspec/changes/ga4-consent-contract`)

## Objective
Implement OpenSpec change `ga4-consent-contract` test-first per its tasks.md (scenario ids there); context in plans/ga4-analytics/00-spec.md.
- W0 also vendors the updated analytics.proto into team-analytics, team-frontend and team-gateway (`proto/` copies; consumers regenerate their own gitignored code).

## Write-set (EXCLUSIVE)
- platform-core/packages/proto/platform/analytics/v1/analytics.proto
- platform-core/packages/proto/go.mod
- openspec/changes/ga4-consent-contract/specs/tracking-consent/spec.md (spec-clarify 2026-10-12, VERIFIED BY lines; re-lock approved by human)
- platform-core/packages/proto/platform/analytics/v1/consent_compat_test.go
- platform-core/packages/proto/platform/analytics/v1/consent_comments_test.go
- platform-core/FEATURES.yaml
- platform-e2e/tests/e2e/features/tracking/consent_contract.feature
- platform-e2e/tests/e2e/step_definitions/consent_contract_steps.py
- platform-e2e/tests/e2e/step_definitions/test_consent_contract.py
- team-analytics/proto/platform/analytics/v1/analytics.proto
- team-frontend/proto/platform/analytics/v1/analytics.proto
- team-gateway/proto/platform/analytics/v1/analytics.proto

## Read-only dependencies
- openspec/changes/ga4-consent-contract/** (frozen by LOCK.md digest)
- plans/ga4-analytics/00-spec.md, plans/ga4-analytics/slices.yaml

## Verify
(cd platform-core/packages/proto && buf lint && buf breaking --against '.git#branch=main,subdir=packages/proto' && buf generate); (cd platform-core/packages/proto && go test ./...)  [proto test file language/location is a guess]; platform-e2e/.venv/bin/python platform-e2e/scripts/spec_sync.py ga4-consent-contract --strict; (cd platform-e2e && HEADLESS=true .venv/bin/python -m pytest -q tests/e2e/step_definitions/test_consent_contract.py)
