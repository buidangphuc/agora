# [W1-T6] payment-refund-audit-id

## Role
feature-run worker (change `openspec/changes/archive/2026-10-22-payment-refund-audit-id`)

## Objective
Implement OpenSpec change `payment-refund-audit-id` test-first per its tasks.md (scenario ids there); context in plans/privacy-audit-hardening/00-spec.md and LOCK.md "Open issues" decisions.
- team-payment admin refund audit event metadata gains refund_id (written before the refund; invalid refund_id writes nothing; retries write a second row with the same refund_id).
- depends_on: none

## Write-set (EXCLUSIVE)
- team-payment/internal/upstream/audit.go
- team-payment/internal/upstream/audit_test.go
- team-payment/internal/handler/payment.go
- team-payment/internal/handler/*_test.go
- team-payment/FEATURES.yaml
- platform-e2e/tests/e2e/features/payment/admin_refund_audit.feature
- platform-e2e/tests/e2e/step_definitions/test_admin_refund_audit.py
- openspec/changes/archive/2026-10-22-payment-refund-audit-id/**

## Read-only dependencies
- openspec/changes/archive/2026-10-22-payment-refund-audit-id/** is frozen by the LOCK.md digest (only task ticks change)
- platform-e2e/tests/e2e/support/audit_support.py (reuse)
- plans/privacy-audit-hardening/00-spec.md, slices.yaml, LOCK.md

## Verify
(cd team-payment && make proto && go vet ./... && go test ./... -race); (cd platform-e2e && HEADLESS=true $PY -m pytest -q tests/e2e/step_definitions/test_admin_refund_audit.py -m 'not destructive') then any @destructive scenario alone; $PY platform-e2e/scripts/spec_sync.py payment-refund-audit-id --strict
