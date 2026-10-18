# [W4-T3] frontend-user-enforcement

## Role
feature-run worker (change `openspec/changes/archive/2026-10-18-frontend-user-enforcement`)

## Objective
Implement OpenSpec change `frontend-user-enforcement` test-first per its tasks.md (scenario ids there); context in plans/q6-trust-enforcement/00-spec.md and the change's design.md Clarifications.
- Admin users page (/admin/users: search, restrict/suspend/ban/reinstate with reason and note), fixed login refusal for suspended/banned users, refresh PermissionDenied clears cookies, cockpit link, restricted-seller notice on seller pages.
- depends_on: identity-user-enforcement

## Write-set (EXCLUSIVE)
- team-frontend/src/generated/platform/identity/**
- team-frontend/src/lib/gateway/client.ts
- team-frontend/src/features/admin/userFilters.ts
- team-frontend/src/features/admin/userFilters.test.ts
- team-frontend/src/lib/gateway/users.ts
- team-frontend/src/lib/gateway/users.test.ts
- team-frontend/src/features/admin/userActions.ts
- team-frontend/src/features/admin/userActions.test.ts
- team-frontend/src/features/admin/UsersView.tsx
- team-frontend/src/features/admin/UsersView.test.tsx
- team-frontend/src/app/(shop)/admin/users/**
- team-frontend/src/features/auth/actions.ts
- team-frontend/src/features/auth/actions.test.ts
- team-frontend/src/middleware.ts
- team-frontend/src/middleware.test.ts
- team-frontend/src/features/admin/CockpitView.tsx
- team-frontend/src/features/admin/CockpitView.test.tsx
- team-frontend/src/app/(shop)/admin/cockpit/page.test.tsx
- team-frontend/src/app/(shop)/seller/layout.tsx
- team-frontend/src/app/(shop)/seller/layout.test.tsx
- team-frontend/src/app/(shop)/seller/restrictedGuard.test.tsx
- team-frontend/src/app/(shop)/seller/(dashboard)/page.tsx
- team-frontend/src/app/(shop)/seller/new/page.tsx
- team-frontend/src/app/(shop)/seller/ads/page.tsx
- team-frontend/src/app/(shop)/seller/plans/page.tsx
- team-frontend/src/app/(shop)/seller/shop/page.tsx
- team-frontend/src/app/(shop)/seller/wallet/page.tsx
- team-frontend/src/app/(shop)/seller/bundles/page.tsx
- team-frontend/src/app/(shop)/seller/analytics/page.tsx
- team-frontend/src/app/(shop)/seller/orders/(list)/page.tsx
- team-frontend/src/app/(shop)/seller/[id]/edit/page.tsx
- team-frontend/FEATURES.yaml
- platform-e2e/tests/e2e/features/frontend/user_enforcement_ui.feature
- platform-e2e/tests/e2e/step_definitions/user_enforcement_steps.py
- platform-e2e/tests/e2e/step_definitions/test_user_enforcement_ui.py
- platform-e2e/tests/e2e/pages/admin_users_page.py
- openspec/changes/archive/2026-10-18-frontend-user-enforcement/**
- team-frontend/src/lib/gateway/client-account-admin.test.ts (# widened at build: task 1.1)

## Read-only dependencies
- openspec/changes/archive/2026-10-18-frontend-user-enforcement/** is frozen by the LOCK.md digest (only task ticks change)
- team-frontend/proto/** (copied by q6-trust-contract; never edit)
- platform-e2e/tests/e2e/support/audit_support.py (never edit)
- platform-e2e/tests/e2e/features/auth/login_return.feature (must stay green)
- plans/q6-trust-enforcement/00-spec.md, slices.yaml, LOCK.md

## Verify
(cd team-frontend && npm run proto && npm test && npm run check); (cd platform-e2e && HEADLESS=true $PY -m pytest -q tests/e2e/step_definitions/test_user_enforcement_ui.py tests/e2e/step_definitions/test_login_return.py -m 'not destructive') then the @destructive audit-down scenario alone; $PY platform-e2e/scripts/spec_sync.py frontend-user-enforcement --strict
