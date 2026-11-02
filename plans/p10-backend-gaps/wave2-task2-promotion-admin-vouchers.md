# [W2-T2] promotion-admin-vouchers

## Role
feature-run worker (change `openspec/changes/promotion-admin-vouchers`)

## Objective
Implement OpenSpec change `promotion-admin-vouchers` test-first per its tasks.md; context in plans/p10-backend-gaps/00-spec.md (Locked + Clarifications) and the change's design.md.
- Admin can stop (DeactivateVoucher) and edit end date/quota (UpdateVoucher) of a voucher with audit-before-write; duplicate code maps to ALREADY_EXISTS; gateway passes ErrorInfo reasons and PROMOTION_UNAVAILABLE; admin UI stop/edit; buyer modal maps `inactive`; remove G9-G11 from gaps.md.

## Write-set (EXCLUSIVE)
- team-promotion/migrations/*voucher*
- team-promotion/internal/store/**
- team-promotion/internal/repository/**
- team-promotion/internal/voucher/**
- team-promotion/internal/store/voucher.go (widened at build: task 1.5)
- team-promotion/internal/voucher/voucher.go (widened at build: task 1.5)
- team-promotion/internal/rpc/voucher.go
- team-promotion/internal/rpc/voucher*_test.go
- team-promotion/internal/rpc/common.go
- team-promotion/internal/rpc/common_test.go
- team-promotion/README.md
- team-promotion/FEATURES.yaml
- team-gateway/internal/edge/promotion.go
- team-gateway/internal/edge/promotion*_test.go
- team-gateway/internal/edge/forward.go
- team-gateway/internal/edge/forward_test.go
- team-frontend-admin/src/lib/gateway/vouchers.ts
- team-frontend-admin/src/lib/gateway/vouchers.test.ts
- team-frontend-admin/src/features/vouchers/**
- team-frontend-admin/src/app/admin/vouchers/**
- team-frontend/src/features/order/VoucherModal.tsx
- team-frontend/src/features/order/VoucherModal.test.tsx
- team-frontend/src/features/voucher/VoucherModal.tsx
- team-frontend/src/features/voucher/VoucherModal.test.tsx
- fe-platform/docs/gaps.md
- platform-e2e/tests/e2e/features/promo/promotion_admin_vouchers.feature
- platform-e2e/tests/e2e/features/frontend/admin_vouchers_ui.feature
- platform-e2e/tests/e2e/step_definitions/promotion_admin_vouchers_steps.py
- platform-e2e/tests/e2e/step_definitions/test_promotion_admin_vouchers.py
- platform-e2e/tests/e2e/step_definitions/admin_vouchers_ui_steps.py
- platform-e2e/tests/e2e/step_definitions/test_admin_vouchers_ui.py
- platform-e2e/tests/e2e/support/admin_vouchers_support.py
- platform-e2e/src/pages/admin_vouchers_page.py
- openspec/changes/promotion-admin-vouchers/**

## Read-only dependencies
- team-promotion CreateVoucher audit.Do path (pattern)
- generated promotion stubs from p10-gaps-contract
- plans/p10-backend-gaps/00-spec.md, LOCK.md

## Verify
PY=/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python; run every check through the gate-digest skill (scripts/asdlc/gate_digest.py).
- make -C team-promotion check
- (cd team-promotion && go test ./... -race) with PROMOTION_TEST_DATABASE_URL set to a throwaway DB (a skip is not a pass)
- (cd team-gateway && go test ./... -race)
- (cd team-frontend-admin && npm test && npm run lint && npx tsc --noEmit)
- (cd team-frontend && npm test -- VoucherModal)
- bash /Users/phuc.buidang/Library/Caches/ai-first-runs/p10-build/e2e.sh <slice-worktree> platform-e2e/tests/e2e/step_definitions/test_promotion_admin_vouchers.py platform-e2e/tests/e2e/step_definitions/test_admin_vouchers_ui.py
- $PY platform-e2e/scripts/spec_sync.py promotion-admin-vouchers --strict
