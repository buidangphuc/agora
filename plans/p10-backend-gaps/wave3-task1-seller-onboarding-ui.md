# [W3-T1] seller-onboarding-ui

## Role
feature-run worker (change `openspec/changes/seller-onboarding-ui`)

## Objective
Implement OpenSpec change `seller-onboarding-ui` test-first per its tasks.md; context in plans/p10-backend-gaps/00-spec.md (Locked + Clarifications) and the change's design.md.
- Seller app onboarding screen for buyers (terms text + version, accept -> BecomeSeller, seller_session cookie update, redirect /seller/new); restricted-only SellerAccessDenied; remove every 'Cần tài khoản Người Bán'; remove G15 from gaps.md.

## Write-set (EXCLUSIVE)
- team-frontend-seller/src/app/seller/layout.tsx
- team-frontend-seller/src/app/seller/layout.test.tsx
- team-frontend-seller/src/features/seller/SellerAccessDenied.tsx
- team-frontend-seller/src/features/seller/SellerOnboarding*.tsx
- team-frontend-seller/src/features/seller/acceptSellerTerms*.ts
- team-frontend-seller/src/features/seller/restrictedGuard*.tsx
- team-frontend-seller/src/features/seller/removedText.test.ts
- team-frontend-seller/src/lib/gateway/session.ts
- team-frontend-seller/src/lib/gateway/session.test.ts
- team-frontend-seller/FEATURES.yaml
- team-frontend-seller/README.md
- fe-platform/docs/gaps.md
- platform-e2e/tests/e2e/features/frontend/seller_onboarding_ui.feature
- platform-e2e/tests/e2e/step_definitions/seller_onboarding_ui_steps.py
- platform-e2e/tests/e2e/step_definitions/test_seller_onboarding_ui.py
- platform-e2e/src/pages/seller_onboarding_page.py
- platform-e2e/tests/e2e/features/frontend/user_enforcement_ui.feature
- platform-e2e/tests/e2e/step_definitions/user_enforcement_steps.py
- platform-e2e/tests/e2e/step_definitions/test_user_enforcement_ui.py
- platform-e2e/tests/e2e/features/auth/login_return.feature
- platform-e2e/tests/e2e/step_definitions/test_login_return.py
- platform-e2e/tests/e2e/step_definitions/seller_cockpit_steps.py
- openspec/changes/seller-onboarding-ui/**
- team-frontend-seller/src/features/seller/onboarding.ts (widened at build: task 1.1)
- team-frontend-seller/src/features/seller/onboardingTerms.ts (widened at build: task 1.1)
- team-frontend-seller/src/features/seller/onboarding.test.ts (widened at build: task 1.2)
- team-frontend-seller/src/app/sell/page.test.ts (widened at build: task 1.3)
- team-frontend-seller/src/app/seller/restrictedGuard.test.tsx (widened at build: task 1.3)
- team-frontend-seller/src/app/seller/removedText.test.ts (widened at build: task 1.6)
- tests/seller-onboarding-ui/docs_2_1.sh (widened at build: task 2.1)

## Read-only dependencies
- identity-seller-onboarding BecomeSeller RPC and gateway route
- vendored identity proto from p10-gaps-contract
- plans/p10-backend-gaps/00-spec.md, LOCK.md

## Verify
PY=/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python; run every check through the gate-digest skill (scripts/asdlc/gate_digest.py).
- (cd team-frontend-seller && npm run vendor:check && npm test && npm run lint && npx tsc --noEmit)
- bash /Users/phuc.buidang/Library/Caches/ai-first-runs/p10-build/e2e.sh <slice-worktree> platform-e2e/tests/e2e/step_definitions/test_seller_onboarding_ui.py platform-e2e/tests/e2e/step_definitions/test_user_enforcement_ui.py platform-e2e/tests/e2e/step_definitions/test_login_return.py
- openspec validate seller-onboarding-ui --strict
- $PY platform-e2e/scripts/spec_sync.py seller-onboarding-ui --strict
