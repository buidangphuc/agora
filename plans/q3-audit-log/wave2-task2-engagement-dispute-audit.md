# [W2-T2] engagement-dispute-audit

## Role
feature-run worker (change `openspec/changes/archive/2026-10-18-engagement-dispute-audit`)

## Objective
Implement OpenSpec change `engagement-dispute-audit` test-first per its tasks.md (scenario ids there); context in plans/q3-audit-log/00-spec.md and the change's design.md Clarifications.
- Reuses the Q2 audit client and `UPSTREAM_AUDIT_ADDR` wiring; no proto, migration, gateway or compose change.

## Write-set (EXCLUSIVE)
- team-engagement/internal/upstream/audit.go
- team-engagement/internal/upstream/audit_test.go
- team-engagement/internal/service/review.go
- team-engagement/internal/service/dispute.go
- team-engagement/internal/service/dispute_test.go
- team-engagement/internal/handler/engagement.go
- team-engagement/internal/grpcserver/server_test.go
- team-engagement/internal/service/review_test.go
- team-engagement/internal/handler/disputes_audit_test.go
- team-engagement/internal/handler/reviews_hide_test.go
- team-engagement/internal/handler/authz_test.go
- team-engagement/internal/handler/engagement_test.go
- team-engagement/cmd/server/main.go
- team-engagement/README.md
- team-engagement/FEATURES.yaml
- platform-e2e/tests/e2e/features/engagement/dispute_audit.feature
- platform-e2e/tests/e2e/step_definitions/test_dispute_audit.py

## Read-only dependencies
- openspec/changes/archive/2026-10-18-engagement-dispute-audit/** (frozen by LOCK.md digest)
- team-engagement/proto/platform/audit/v1/audit.proto (vendored by q3-audit-log-contract; regenerate gitignored `generated/` with `make proto`, never hand-edit)
- platform-e2e/tests/e2e/support/audit_support.py (audit-query-filters output; import only, never edit)
- platform-e2e/tests/e2e/step_definitions/group_b_steps.py, platform-e2e/tests/e2e/step_definitions/disputes_access_steps.py (dispute steps to reuse)
- plans/q3-audit-log/00-spec.md, plans/q3-audit-log/slices.yaml, plans/q3-audit-log/LOCK.md

## Verify
(cd team-engagement && make proto && go test ./...); (cd platform-e2e && HEADLESS=true /Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python -m pytest -q tests/e2e/features/engagement/dispute_audit.feature); platform-e2e/.venv/bin/python platform-e2e/scripts/spec_sync.py engagement-dispute-audit --strict
