# [W3-T2] frontend-welcome-checklist

## Role
feature-run worker (change `openspec/changes/frontend-welcome-checklist`)

## Objective
Implement OpenSpec change `frontend-welcome-checklist` test-first per its tasks.md; context in plans/q7-q37-referral-welcome/00-spec.md and LOCK.md (banner "Voucher đang được chuẩn bị" only while the account is < 1 hour old).
- "Voucher của bạn" on /vouchers, own vouchers in the checkout voucher modal, WelcomePanel (banner + 3-step checklist) on home and account shell, dismiss cookies.
- depends_on: promotion-new-user-vouchers (runtime only; code depends on the merged contract), frontend-referral-invite (merged)

## Write-set (EXCLUSIVE)
- team-frontend/src/lib/gateway/promotion.ts
- team-frontend/src/lib/gateway/promotion.test.ts
- team-frontend/src/features/voucher/**
- team-frontend/src/features/order/VoucherModal.tsx
- team-frontend/src/features/order/VoucherModal.test.tsx
- team-frontend/src/features/order/CheckoutSteps.tsx
- team-frontend/src/features/order/CheckoutSteps.test.tsx
- team-frontend/src/app/(checkout)/checkout/(summary)/**
- team-frontend/src/features/welcome/**
- team-frontend/src/app/(shop)/vouchers/**
- team-frontend/src/app/(shop)/(home)/page.tsx
- team-frontend/src/app/(shop)/(home)/page.test.tsx
- team-frontend/src/features/account/AccountShell.tsx
- team-frontend/src/features/account/AccountShell.test.tsx
- team-frontend/FEATURES.yaml
- team-frontend/README.md
- platform-e2e/tests/e2e/features/frontend/welcome_checklist.feature
- platform-e2e/tests/e2e/step_definitions/welcome_checklist_steps.py
- platform-e2e/tests/e2e/step_definitions/test_welcome_checklist.py

## Verify
- `cd team-frontend && npm run proto && npm run check && npm run build`
- e2e: deferred
