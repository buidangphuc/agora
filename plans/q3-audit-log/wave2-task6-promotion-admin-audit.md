# [W2-T6] promotion-admin-audit

## Role
feature-run worker (change `openspec/changes/archive/2026-10-18-promotion-admin-audit`)

## Objective
Implement OpenSpec change `promotion-admin-audit` test-first per its tasks.md (scenario ids there); context in plans/q3-audit-log/00-spec.md and the change's design.md Clarifications.
- Audits platform vouchers and admin flash-sale / ad campaigns before insert; seller paths unchanged.

## Write-set (EXCLUSIVE)
- team-promotion/internal/handler/grpc_test.go
- team-promotion/internal/config/config.go
- team-promotion/internal/config/config_test.go
- team-promotion/.env.example
- team-promotion/README.md
- team-promotion/internal/upstream/audit.go
- team-promotion/internal/upstream/audit_test.go
- team-promotion/internal/service/audit.go
- team-promotion/internal/service/voucher.go
- team-promotion/internal/service/flashsale.go
- team-promotion/internal/service/sponsored.go
- team-promotion/internal/service/voucher_test.go
- team-promotion/internal/service/flashsale_test.go
- team-promotion/internal/service/sponsored_test.go
- team-promotion/internal/handler/voucher.go
- team-promotion/internal/handler/flashsale.go
- team-promotion/internal/handler/sponsored.go
- team-promotion/internal/handler/audit_test.go
- team-promotion/cmd/server/main.go
- team-promotion/FEATURES.yaml
- platform-e2e/tests/e2e/features/promo/promotion_admin_audit.feature
- platform-e2e/tests/e2e/step_definitions/test_promotion_admin_audit.py

## Read-only dependencies
- openspec/changes/archive/2026-10-18-promotion-admin-audit/** (frozen by LOCK.md digest)
- team-promotion/proto/platform/audit/v1/audit.proto (vendored by q3-audit-log-contract; regenerate gitignored `generated/` with `make proto`, never hand-edit)
- platform-e2e/tests/e2e/support/audit_support.py (audit-query-filters output; import only, never edit)
- platform-e2e/tests/e2e/step_definitions/flash_sale_ownership_steps.py (reuse only)
- team-promotion/internal/upstream/timeout.go (helper style)
- plans/q3-audit-log/00-spec.md, plans/q3-audit-log/slices.yaml, plans/q3-audit-log/LOCK.md

## Verify
(cd team-promotion && make proto && go test ./...); (cd platform-e2e && HEADLESS=true /Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python -m pytest -q tests/e2e/features/promo/promotion_admin_audit.feature); platform-e2e/.venv/bin/python platform-e2e/scripts/spec_sync.py promotion-admin-audit --strict
