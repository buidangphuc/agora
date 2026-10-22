# [W3-T1] frontend-referrer-hygiene

## Role
feature-run worker (change `openspec/changes/archive/2026-10-22-frontend-referrer-hygiene`)

## Objective
Implement OpenSpec change `frontend-referrer-hygiene` test-first per its tasks.md (scenario ids there); context in plans/privacy-audit-hardening/00-spec.md and LOCK.md "Open issues" decisions.
- Referrer-Policy strict-origin-when-cross-origin on every response; no-referrer metadata on login and every admin page; admin page_location/page_referrer path-only in GTM pushes; admin returnUrl path-only.
- depends_on: frontend-cookie-secure, frontend-session-client-context (wave move: shared team-frontend/FEATURES.yaml)

## Write-set (EXCLUSIVE)
- team-frontend/next.config.mjs
- team-frontend/next.config.test.ts
- team-frontend/src/app/(shop)/login/page.tsx
- team-frontend/src/app/(shop)/admin/cockpit/page.tsx
- team-frontend/src/app/(shop)/admin/audit/page.tsx
- team-frontend/src/app/(shop)/admin/listings/page.tsx
- team-frontend/src/app/(shop)/admin/cases/page.tsx
- team-frontend/src/app/(shop)/admin/users/page.test.tsx
- team-frontend/src/app/(shop)/admin/cases/page.test.tsx
- team-frontend/src/app/referrerMetadata.test.ts
- team-frontend/src/lib/analytics/destinations/gtm.ts
- team-frontend/src/lib/analytics/destinations/gtm.test.ts
- team-frontend/src/features/auth/guardPath.ts
- team-frontend/src/features/auth/guardPath.test.ts
- team-frontend/FEATURES.yaml
- team-frontend/e2e/referrer-hygiene.spec.ts
- platform-e2e/tests/e2e/features/frontend/referrer_hygiene_ui.feature
- platform-e2e/tests/e2e/step_definitions/test_referrer_hygiene_ui.py
- openspec/changes/archive/2026-10-22-frontend-referrer-hygiene/**

## Read-only dependencies
- openspec/changes/archive/2026-10-22-frontend-referrer-hygiene/** is frozen by the LOCK.md digest (only task ticks change)
- team-frontend/src/lib/analytics/scrub.ts (reuse scrubReferrer)
- platform-e2e/tests/e2e/features/auth/login_return.feature and test_login_return.py (keep untouched and green)
- plans/privacy-audit-hardening/00-spec.md, slices.yaml, LOCK.md

## Verify
(cd team-frontend && npm run proto && npm test && npm run check); (cd platform-e2e && HEADLESS=true $PY -m pytest -q tests/e2e/step_definitions/test_referrer_hygiene_ui.py tests/e2e/step_definitions/test_login_return.py); $PY platform-e2e/scripts/spec_sync.py frontend-referrer-hygiene --strict
