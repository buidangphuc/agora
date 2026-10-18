# [W2-T3] frontend-session-client-context

## Role
feature-run worker (change `openspec/changes/frontend-session-client-context`)

## Objective
Implement OpenSpec change `frontend-session-client-context` test-first per its tasks.md (scenario ids there); context in plans/privacy-audit-hardening/00-spec.md and LOCK.md "Open issues" decisions.
- Refresh rotation records client IP/UA (COALESCE on empty) in team-identity; team-frontend forwards client context on middleware refresh, logout refresh/revoke and revoke-another-device.
- depends_on: frontend-cookie-secure

## Write-set (EXCLUSIVE)
- team-identity/internal/repository/session.go
- team-identity/internal/repository/*_test.go
- team-identity/internal/service/auth.go
- team-identity/internal/service/refresh.go
- team-identity/internal/service/auth_refresh_test.go
- team-identity/internal/handler/auth_test.go
- team-identity/README.md
- team-identity/FEATURES.yaml
- team-frontend/src/lib/gateway/refresh.ts
- team-frontend/src/lib/gateway/refresh.test.ts
- team-frontend/src/lib/gateway/revoke.ts
- team-frontend/src/lib/gateway/revoke.test.ts
- team-frontend/src/lib/gateway/sessions.ts
- team-frontend/src/lib/gateway/sessions.test.ts
- team-frontend/src/lib/gateway/client-context.ts
- team-frontend/src/lib/gateway/client-context.test.ts
- team-frontend/src/middleware.ts
- team-frontend/src/middleware.test.ts
- team-frontend/src/features/auth/actions.ts
- team-frontend/src/features/auth/actions.test.ts
- team-frontend/FEATURES.yaml
- platform-e2e/tests/e2e/features/auth/session_client_context.feature
- platform-e2e/tests/e2e/step_definitions/test_session_client_context.py
- openspec/changes/frontend-session-client-context/**

## Read-only dependencies
- openspec/changes/frontend-session-client-context/** is frozen by the LOCK.md digest (only task ticks change)
- platform-e2e/tests/e2e/step_definitions/session_refresh_steps.py, session_revocation_steps.py (reuse)
- plans/privacy-audit-hardening/00-spec.md, slices.yaml, LOCK.md

## Verify
(cd team-identity && make proto && go vet ./... && go test ./... -race) with TEST_DATABASE_URL on a throwaway postgres; (cd team-frontend && npm run proto && npm test && npm run check); (cd platform-e2e && HEADLESS=true $PY -m pytest -q tests/e2e/step_definitions/test_session_client_context.py); make -C platform-e2e spec-check CHANGE=frontend-session-client-context; $PY platform-e2e/scripts/spec_sync.py frontend-session-client-context --strict
