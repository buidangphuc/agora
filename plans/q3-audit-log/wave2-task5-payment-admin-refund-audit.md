# [W2-T5] payment-admin-refund-audit

## Role
feature-run worker (change `openspec/changes/payment-admin-refund-audit`)

## Objective
Implement OpenSpec change `payment-admin-refund-audit` test-first per its tasks.md (scenario ids there); context in plans/q3-audit-log/00-spec.md and the change's design.md Clarifications.
- Audits the admin branch of RefundPayment before money moves; the seller branch is untouched.

## Write-set (EXCLUSIVE)
- team-payment/internal/handler/payment_test.go
- team-payment/internal/upstream/audit.go
- team-payment/internal/upstream/audit_test.go
- team-payment/internal/config/config.go
- team-payment/internal/config/config_test.go
- team-payment/internal/bootstrap/env_drift_test.go
- team-payment/cmd/server/main.go
- team-payment/.env.example
- team-payment/README.md
- team-payment/internal/handler/payment.go
- team-payment/internal/handler/refund_rpc_test.go
- team-payment/internal/handler/refund_audit_test.go
- platform-e2e/tests/e2e/support/prm_pay_stack.py
- team-payment/FEATURES.yaml
- platform-e2e/tests/e2e/features/payment/admin_refund_audit.feature
- platform-e2e/tests/e2e/step_definitions/test_admin_refund_audit.py

## Read-only dependencies
- openspec/changes/payment-admin-refund-audit/** (frozen by LOCK.md digest)
- team-payment/proto/platform/audit/v1/audit.proto (vendored by q3-audit-log-contract; regenerate gitignored `generated/` with `make proto`, never hand-edit)
- platform-e2e/tests/e2e/support/audit_support.py (audit-query-filters output; import only, never edit)
- team-engagement/internal/upstream/audit.go (client to mirror)
- platform-e2e/tests/e2e/features/payment/cumulative_refunds.feature and its steps (reuse only)
- plans/q3-audit-log/00-spec.md, plans/q3-audit-log/slices.yaml, plans/q3-audit-log/LOCK.md

## Verify
(cd team-payment && make proto && go test ./...); (cd platform-e2e && HEADLESS=true /Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python -m pytest -q tests/e2e/features/payment/admin_refund_audit.feature); platform-e2e/.venv/bin/python platform-e2e/scripts/spec_sync.py payment-admin-refund-audit --strict
