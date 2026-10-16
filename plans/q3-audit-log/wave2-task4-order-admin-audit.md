# [W2-T4] order-admin-audit

## Role
feature-run worker (change `openspec/changes/order-admin-audit`)

## Objective
Implement OpenSpec change `order-admin-audit` test-first per its tasks.md (scenario ids there); context in plans/q3-audit-log/00-spec.md and the change's design.md Clarifications.
- Audits ForceFailSaga, UpdateOrderStatus, UpdateReturnStatus and CreateShipment on the `order.admin` path; the proto copy is the contract slice's (only regenerate).

## Write-set (EXCLUSIVE)
- team-order/.env.example
- team-order/internal/handler/order_admin_scope_test.go
- team-order/internal/handler/return_refund_test.go
- team-order/internal/handler/shipment_status_test.go
- team-order/internal/upstream/audit.go
- team-order/internal/upstream/audit_test.go
- team-order/internal/config/config.go
- team-order/internal/config/config_test.go
- team-order/cmd/server/main.go
- team-order/README.md
- team-order/internal/handler/order.go
- team-order/internal/handler/order_admin_audit_test.go
- team-order/internal/handler/order_test.go
- team-order/internal/handler/saga_view_test.go
- team-order/FEATURES.yaml
- platform-e2e/tests/e2e/features/order/admin_audit.feature
- platform-e2e/tests/e2e/step_definitions/test_order_admin_audit.py

## Read-only dependencies
- openspec/changes/order-admin-audit/** (frozen by LOCK.md digest)
- team-order/proto/platform/audit/v1/audit.proto (vendored by q3-audit-log-contract; regenerate gitignored `generated/` with `make proto`, never hand-edit)
- platform-e2e/tests/e2e/support/audit_support.py (audit-query-filters output; import only, never edit)
- platform-e2e/tests/e2e/support/oic_order_support.py (reuse only)
- team-order/internal/handler/order_admin_scope_test.go (pattern to mirror)
- plans/q3-audit-log/00-spec.md, plans/q3-audit-log/slices.yaml, plans/q3-audit-log/LOCK.md

## Verify
(cd team-order && make proto && go test ./...); (cd platform-e2e && HEADLESS=true /Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python -m pytest -q tests/e2e/features/order/admin_audit.feature); platform-e2e/.venv/bin/python platform-e2e/scripts/spec_sync.py order-admin-audit --strict
