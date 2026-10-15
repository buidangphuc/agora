# [W2-T7] verification-kyc-audit

## Role
feature-run worker (change `openspec/changes/verification-kyc-audit`)

## Objective
Implement OpenSpec change `verification-kyc-audit` test-first per its tasks.md (scenario ids there); context in plans/q3-audit-log/00-spec.md and the change's design.md Clarifications.
- Audits admin KYC review (`kyc.review`) before the decision is applied; adds team-verification's first upstream client.

## Write-set (EXCLUSIVE)
- team-verification/internal/config/config.go
- team-verification/internal/config/config_test.go
- team-verification/internal/upstream/audit.go
- team-verification/internal/upstream/audit_test.go
- team-verification/go.mod
- team-verification/go.sum
- team-verification/internal/handler/verification.go
- team-verification/internal/service/verification.go
- team-verification/internal/handler/verification_test.go
- team-verification/internal/handler/verification_audit_test.go
- team-verification/cmd/server/main.go
- team-verification/.env.example
- team-verification/README.md
- team-verification/FEATURES.yaml
- platform-e2e/tests/e2e/features/services/verification_kyc_audit.feature
- platform-e2e/tests/e2e/step_definitions/verification_kyc_audit_steps.py
- platform-e2e/tests/e2e/step_definitions/test_verification_kyc_audit.py

## Read-only dependencies
- openspec/changes/verification-kyc-audit/** (frozen by LOCK.md digest)
- team-verification/proto/platform/audit/v1/audit.proto (vendored by q3-audit-log-contract; regenerate gitignored `generated/` with `make proto`, never hand-edit)
- platform-e2e/tests/e2e/support/audit_support.py (audit-query-filters output; import only, never edit)
- team-engagement/internal/upstream/audit.go (client to mirror)
- platform-e2e/tests/e2e/step_definitions/verification_access_steps.py (`_register_and_submit` pattern, reuse only)
- plans/q3-audit-log/00-spec.md, plans/q3-audit-log/slices.yaml, plans/q3-audit-log/LOCK.md

## Verify
(cd team-verification && make proto && go test ./...); (cd platform-e2e && HEADLESS=true /Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python -m pytest -q tests/e2e/features/services/verification_kyc_audit.feature); platform-e2e/.venv/bin/python platform-e2e/scripts/spec_sync.py verification-kyc-audit --strict
