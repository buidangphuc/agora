# [W1-T4] frontend-cookie-secure

## Role
feature-run worker (change `openspec/changes/archive/2026-10-22-frontend-cookie-secure`)

## Objective
Implement OpenSpec change `frontend-cookie-secure` test-first per its tasks.md (scenario ids there); context in plans/privacy-audit-hardening/00-spec.md and LOCK.md "Open issues" decisions.
- COOKIE_SECURE=true (trimmed, case-insensitive, read per request) forces Secure on session, agora_refresh and agora_auth_evt; unset keeps the x-forwarded-proto rule; staging/prod gitops set it, local compose does not.
- depends_on: none

## Write-set (EXCLUSIVE)
- team-frontend/src/lib/gateway/session-cookie.ts
- team-frontend/src/lib/gateway/session-cookie.test.ts
- team-frontend/src/features/auth/actions.ts
- team-frontend/src/features/auth/actions.test.ts
- team-frontend/src/middleware.ts
- team-frontend/src/middleware.test.ts
- team-frontend/.env.example
- team-frontend/README.md
- team-frontend/FEATURES.yaml
- platform-gitops/envs/staging/services/team-frontend.yaml
- platform-gitops/envs/prod/services/team-frontend.yaml
- platform-gitops/tests/test_frontend_cookie_secure_manifests.py
- platform-e2e/tests/e2e/features/auth/cookie_secure.feature
- platform-e2e/tests/e2e/step_definitions/test_cookie_secure.py
- openspec/changes/archive/2026-10-22-frontend-cookie-secure/**
- team-frontend/src/docs/cookie-secure-docs.test.ts (# widened at build: task 1.5)
- plans/privacy-audit-hardening/wave1-task4-frontend-cookie-secure.md (# widened at build: task 1.5; packet widening record)

## Read-only dependencies
- openspec/changes/archive/2026-10-22-frontend-cookie-secure/** is frozen by the LOCK.md digest (only task ticks change)
- docker-compose.services.yaml, platform-e2e/docker-compose.e2e.yaml, platform-gitops/envs/local/values.yaml (must stay without COOKIE_SECURE)
- plans/privacy-audit-hardening/00-spec.md, slices.yaml, LOCK.md

## Verify
(cd team-frontend && npm run proto && npm test && npm run check); (cd platform-gitops && $PY -m pytest -q tests/test_frontend_cookie_secure_manifests.py); (cd platform-e2e && HEADLESS=true $PY -m pytest -q tests/e2e/step_definitions/test_cookie_secure.py); $PY platform-e2e/scripts/spec_sync.py frontend-cookie-secure --strict
