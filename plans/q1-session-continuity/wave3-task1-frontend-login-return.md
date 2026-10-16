# [W3-T1] frontend-login-return

## Role
feature-run worker (change `openspec/changes/frontend-login-return`)

## Objective
Implement OpenSpec change `frontend-login-return` test-first per its tasks.md (scenario ids there); context in plans/q1-session-continuity/00-spec.md (incl. Amendments) and the change's design.md Clarifications.
- Rebases on frontend-session-continuity (shared actions.ts; consumes x-pathname). Every `redirect("/login")` under team-frontend/src/app is replaced via the helper.

## Write-set (EXCLUSIVE)
- team-frontend/src/features/auth/returnUrl.ts
- team-frontend/src/features/auth/returnUrlShared.ts
- team-frontend/src/features/auth/guardPath.ts
- team-frontend/src/features/tracking/AnalyticsProvider.tsx (navigation hook for trackPageLocation)
- team-frontend/src/features/auth/returnUrl.test.ts
- team-frontend/src/features/auth/actions.ts
- team-frontend/src/features/auth/actions.test.ts
- team-frontend/src/features/auth/LoginForm.tsx
- team-frontend/src/features/auth/RegisterForm.tsx
- team-frontend/src/features/auth/forms.test.tsx
- team-frontend/src/app/guards.test.ts
- team-frontend/src/directives.test.ts
- team-frontend/src/app/**/*.test.ts(x) (new test files for this slice)
- team-frontend/src/features/**/*.test.ts(x) (new or extended test files for this slice)
- team-frontend/src/app/(shop)/**/page.tsx (guard lines only)
- team-frontend/src/app/(shop)/**/layout.tsx (guard lines only)
- team-frontend/src/app/(checkout)/**/page.tsx (guard lines only)
- team-frontend/src/app/(shop)/login/page.tsx
- team-frontend/src/app/(shop)/register/page.tsx
- team-frontend/src/features/cart/actions.ts
- team-frontend/src/features/cart/PurchaseContext.tsx
- team-frontend/src/features/cart/PurchasePanel.tsx
- team-frontend/src/features/cart/PurchasePanel.test.tsx
- team-frontend/src/features/chat/ChatWithSellerButton.tsx
- team-frontend/src/features/listing/qa/QASection.tsx
- team-frontend/src/features/listing/qa/QuestionList.tsx
- team-frontend/src/lib/analytics/destinations/gtm.ts
- team-frontend/src/lib/analytics/destinations/gtm.test.ts
- team-frontend/analytics/gtm-container.json (human decision 2026-10-14: map page_location)
- team-frontend/src/lib/analytics/destinations/gtm-container.test.ts (container mapping test)
- team-frontend/FEATURES.yaml
- platform-e2e/tests/e2e/features/auth/login_return*.feature
- platform-e2e/tests/e2e/step_definitions/login_return*_steps.py
- platform-e2e/tests/e2e/step_definitions/test_login_return*.py
- platform-e2e/src/pages/** (new page objects only)

## Read-only dependencies
- openspec/changes/frontend-login-return/** (frozen by LOCK.md digest)
- plans/q1-session-continuity/00-spec.md, plans/q1-session-continuity/slices.yaml, plans/q1-session-continuity/LOCK.md (reviewer notes)

## Verify
(cd team-frontend && npx vitest run && npm run typecheck); platform-e2e/.venv/bin/python platform-e2e/scripts/spec_sync.py frontend-login-return --strict; (cd platform-e2e && HEADLESS=true .venv/bin/python -m pytest -q tests/e2e/step_definitions/test_login_return*.py)
