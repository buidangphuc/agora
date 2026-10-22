# [W1-T5] identity-session-retention

## Role
feature-run worker (change `openspec/changes/archive/2026-10-22-identity-session-retention`)

## Objective
Implement OpenSpec change `identity-session-retention` test-first per its tasks.md (scenario ids there); context in plans/privacy-audit-hardening/00-spec.md and LOCK.md "Open issues" decisions.
- team-identity purges revoked/expired sessions after SESSION_RETENTION and login history after LOGIN_HISTORY_RETENTION with a batched SessionSweeper (migration 0008_session_revoked_at, counts-only logs, drained on shutdown).
- depends_on: none

## Write-set (EXCLUSIVE)
- team-identity/migrations/0008_session_revoked_at.up.sql
- team-identity/migrations/0008_session_revoked_at.down.sql
- team-identity/internal/repository/session.go
- team-identity/internal/repository/users_pg.go (widened at build: task 1)
- team-identity/internal/repository/*_test.go
- team-identity/internal/service/session_sweeper.go
- team-identity/internal/service/session_sweeper_test.go
- team-identity/internal/config/config.go
- team-identity/internal/config/config_test.go
- team-identity/internal/config/session_retention_test.go (widened at build: task 4)
- team-identity/internal/bootstrap/lifecycle.go
- team-identity/internal/bootstrap/*_test.go
- team-identity/.env.example
- team-identity/README.md
- team-identity/FEATURES.yaml
- platform-e2e/compose/session-retention.override.yaml
- platform-e2e/compose/README.md
- platform-e2e/tests/e2e/features/auth/session_retention.feature
- platform-e2e/tests/e2e/step_definitions/test_session_retention.py
- openspec/changes/archive/2026-10-22-identity-session-retention/**

## Read-only dependencies
- openspec/changes/archive/2026-10-22-identity-session-retention/** is frozen by the LOCK.md digest (only task ticks change)
- platform-e2e/tests/e2e/step_definitions/session_revocation_steps.py, session_refresh_steps.py (reuse)
- plans/privacy-audit-hardening/00-spec.md, slices.yaml, LOCK.md

## Verify
(cd team-identity && make proto && go vet ./... && go test ./... -race) with TEST_DATABASE_URL on a throwaway postgres; (cd platform-e2e && HEADLESS=true $PY -m pytest -q tests/e2e/step_definitions/test_session_retention.py) with team-identity started under compose/session-retention.override.yaml (default-stack control scenario without it); $PY platform-e2e/scripts/spec_sync.py identity-session-retention --strict
