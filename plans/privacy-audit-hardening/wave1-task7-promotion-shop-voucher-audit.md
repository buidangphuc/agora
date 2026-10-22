# [W1-T7] promotion-shop-voucher-audit

## Role
feature-run worker (change `openspec/changes/promotion-shop-voucher-audit`)

## Objective
Implement OpenSpec change `promotion-shop-voucher-audit` test-first per its tasks.md (scenario ids there); context in plans/privacy-audit-hardening/00-spec.md and LOCK.md "Open issues" decisions.
- team-promotion audits an admin-created SHOP voucher (voucher.admin_create, metadata discount_type, value, seller_id) before insert; audit down refuses it; seller shop vouchers are not audited.
- depends_on: pah-contract

## Write-set (EXCLUSIVE)
- team-promotion/internal/service/voucher.go
- team-promotion/internal/service/voucher_test.go
- team-promotion/internal/handler/voucher.go
- team-promotion/internal/handler/audit_test.go
- team-promotion/README.md
- team-promotion/FEATURES.yaml
- platform-e2e/tests/e2e/features/promo/promotion_shop_voucher_audit.feature
- platform-e2e/tests/e2e/step_definitions/test_promotion_shop_voucher_audit.py
- openspec/changes/promotion-shop-voucher-audit/**
- team-promotion/internal/docs/readme_test.go (# widened at build: task 3)
- plans/privacy-audit-hardening/wave1-task7-promotion-shop-voucher-audit.md (# widened at build: task 3; packet widening record)

## Read-only dependencies
- openspec/changes/promotion-shop-voucher-audit/** is frozen by the LOCK.md digest (only task ticks change)
- team-promotion/proto/platform/audit/v1/audit.proto (copied by pah-contract)
- platform-e2e/tests/e2e/support/audit_support.py (reuse)
- plans/privacy-audit-hardening/00-spec.md, slices.yaml, LOCK.md

## Verify
(cd team-promotion && make proto && go vet ./... && go test ./... -race) with PROMOTION_TEST_DATABASE_URL on a throwaway postgres; (cd platform-e2e && HEADLESS=true $PY -m pytest -q tests/e2e/step_definitions/test_promotion_shop_voucher_audit.py -m 'not destructive') then the @destructive audit-down scenarios alone; $PY platform-e2e/scripts/spec_sync.py promotion-shop-voucher-audit --strict
