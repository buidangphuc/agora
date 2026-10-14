# [W1-T1] identity-session-refresh

## Role
feature-run worker (change `openspec/changes/identity-session-refresh`)

## Objective
Implement OpenSpec change `identity-session-refresh` test-first per its tasks.md (scenario ids there); context in plans/q1-session-continuity/00-spec.md (incl. Amendments) and the change's design.md Clarifications.
- Regenerate gitignored code in team-identity and team-gateway from their vendored proto first.

## Write-set (EXCLUSIVE)
- team-identity/migrations/0006_session_refresh.up.sql
- team-identity/migrations/0006_session_refresh.down.sql
- team-identity/internal/repository/session.go
- team-identity/internal/repository/session_refresh_pg_test.go
- team-identity/internal/repository/session_test.go
- team-identity/internal/repository/session_refresh_test.go
- team-identity/internal/repository/*_test.go (new test files for this slice)
- team-identity/internal/service/*_test.go (new test files for this slice)
- team-identity/internal/handler/*_test.go (new test files for this slice)
- team-gateway/internal/edge/*_test.go (new test files for this slice)
- team-identity/internal/service/auth.go
- team-identity/internal/service/refresh.go
- team-identity/internal/service/auth_refresh_test.go
- team-identity/internal/handler/auth.go
- team-identity/internal/handler/auth_refresh_test.go
- team-identity/internal/handler/session.go
- team-identity/internal/handler/session_test.go
- team-identity/internal/authz/scopes.go
- team-identity/internal/authz/coverage_test.go
- team-identity/internal/authz/*_test.go (new test files for this slice)
- team-identity/internal/config/config.go
- team-identity/FEATURES.yaml
- team-gateway/internal/edge/auth.go
- team-gateway/internal/edge/auth_test.go
- team-gateway/internal/edge/unrouted_test.go
- team-gateway/internal/edge/server.go
- team-gateway/FEATURES.yaml
- platform-e2e/tests/e2e/features/auth/session_refresh*.feature
- platform-e2e/tests/e2e/step_definitions/session_refresh*_steps.py
- platform-e2e/tests/e2e/step_definitions/test_session_refresh*.py
- platform-e2e/src/pages/** (new page objects only)

## Read-only dependencies
- openspec/changes/identity-session-refresh/** (frozen by LOCK.md digest)
- plans/q1-session-continuity/00-spec.md, plans/q1-session-continuity/slices.yaml, plans/q1-session-continuity/LOCK.md (reviewer notes)

## Verify
(cd team-identity && go vet ./... && go test ./...); (cd team-gateway && go vet ./... && go test ./...); platform-e2e/.venv/bin/python platform-e2e/scripts/spec_sync.py identity-session-refresh --strict; (cd platform-e2e && HEADLESS=true .venv/bin/python -m pytest -q tests/e2e/step_definitions/test_session_refresh*.py)
