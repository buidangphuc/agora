# [W2-T1] frontend-session-continuity

## Role
feature-run worker (change `openspec/changes/archive/2026-10-18-frontend-session-continuity`)

## Objective
Implement OpenSpec change `frontend-session-continuity` test-first per its tasks.md (scenario ids there); context in plans/q1-session-continuity/00-spec.md (incl. Amendments) and the change's design.md Clarifications.
- Middleware moves to the Node runtime and also sets the `x-pathname` request header consumed by frontend-login-return.

## Write-set (EXCLUSIVE)
- team-frontend/src/middleware.ts
- team-frontend/src/middleware.test.ts
- team-frontend/src/lib/gateway/session-cookie.ts
- team-frontend/src/lib/gateway/session.ts
- team-frontend/src/lib/gateway/refresh.ts
- team-frontend/src/lib/gateway/revoke.ts
- team-frontend/src/lib/gateway/refresh.test.ts
- team-frontend/src/lib/gateway/*.test.ts (new test files for this slice)
- team-frontend/src/features/auth/actions.ts
- team-frontend/src/features/auth/actions.test.ts
- team-frontend/FEATURES.yaml
- platform-e2e/tests/e2e/features/auth/session_continuity*.feature
- platform-e2e/tests/e2e/step_definitions/session_continuity*_steps.py
- platform-e2e/tests/e2e/step_definitions/test_session_continuity*.py
- platform-e2e/src/pages/** (new page objects only)

## Read-only dependencies
- openspec/changes/archive/2026-10-18-frontend-session-continuity/** (frozen by LOCK.md digest)
- plans/q1-session-continuity/00-spec.md, plans/q1-session-continuity/slices.yaml, plans/q1-session-continuity/LOCK.md (reviewer notes)

## Verify
(cd team-frontend && npx vitest run src/middleware.test.ts src/features/auth src/lib/gateway && npm run typecheck); platform-e2e/.venv/bin/python platform-e2e/scripts/spec_sync.py frontend-session-continuity --strict; (cd platform-e2e && HEADLESS=true .venv/bin/python -m pytest -q tests/e2e/step_definitions/test_session_continuity*.py)
