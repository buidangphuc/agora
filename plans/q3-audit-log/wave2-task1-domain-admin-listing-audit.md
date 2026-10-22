# [W2-T1] domain-admin-listing-audit

## Role
feature-run worker (change `openspec/changes/archive/2026-10-18-domain-admin-listing-audit`)

## Objective
Implement OpenSpec change `domain-admin-listing-audit` test-first per its tasks.md (scenario ids there); context in plans/q3-audit-log/00-spec.md and the change's design.md Clarifications.
- New audit upstream client in team-domain; the root compose address is owned by the contract slice (only `team-domain/docker-compose.local.yaml` is edited here).

## Write-set (EXCLUSIVE)
- team-domain/internal/upstream/audit.go
- team-domain/internal/upstream/audit_test.go
- team-domain/internal/upstream/timeout.go
- team-domain/internal/config/config.go
- team-domain/internal/config/config_test.go
- team-domain/.env.example
- team-domain/README.md
- team-domain/Makefile
- team-domain/internal/service/listing.go
- team-domain/internal/service/listing_test.go
- team-domain/internal/handler/listing.go
- team-domain/internal/handler/listing_audit_test.go
- team-domain/cmd/server/main.go
- team-domain/internal/bootstrap/resources.go
- team-domain/internal/bootstrap/lifecycle.go
- team-domain/docker-compose.local.yaml
- team-domain/proto/README.md
- team-domain/FEATURES.yaml
- platform-e2e/tests/e2e/features/seller/listing_admin_audit.feature
- platform-e2e/tests/e2e/step_definitions/test_listing_admin_audit.py

## Read-only dependencies
- openspec/changes/archive/2026-10-18-domain-admin-listing-audit/** (frozen by LOCK.md digest)
- team-domain/proto/platform/audit/v1/audit.proto (vendored by q3-audit-log-contract; regenerate gitignored `generated/` with `make proto`, never hand-edit)
- platform-e2e/tests/e2e/support/audit_support.py (audit-query-filters output; import only, never edit)
- team-domain/internal/handler/listing_test.go, team-domain/internal/handler/listing_visibility_test.go (principal helpers to mirror)
- plans/q3-audit-log/00-spec.md, plans/q3-audit-log/slices.yaml, plans/q3-audit-log/LOCK.md

## Verify
(cd team-domain && make proto && go test ./...); (cd platform-e2e && HEADLESS=true /Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python -m pytest -q tests/e2e/features/seller/listing_admin_audit.feature); platform-e2e/.venv/bin/python platform-e2e/scripts/spec_sync.py domain-admin-listing-audit --strict
