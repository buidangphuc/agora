# [W0-T1] pah-contract

## Role
feature-run worker (change `openspec/changes/archive/2026-10-22-pah-contract`)

## Objective
Implement OpenSpec change `pah-contract` test-first per its tasks.md (scenario ids there); context in plans/privacy-audit-hardening/00-spec.md and LOCK.md "Open issues" decisions.
- Additive contract: engagement.events ReviewHidden (no user data, keyed by listing id) after ReviewCreated; audit catalogue comment to 18 actions (voucher.admin_create); byte-identical copies into the consumer repos.
- depends_on: none

## Write-set (EXCLUSIVE)
- platform-core/packages/proto/pah_contract_test.sh
- platform-core/packages/proto/platform/engagement/v1/events.proto
- platform-core/packages/proto/platform/audit/v1/audit.proto
- platform-core/packages/proto/platform/engagement/v1/*_test.go
- platform-core/packages/proto/platform/audit/v1/*_test.go
- platform-core/gen/**
- platform-core/FEATURES.yaml
- team-engagement/proto/platform/engagement/v1/events.proto
- team-analytics/proto/platform/engagement/v1/events.proto
- team-order/proto/platform/engagement/v1/events.proto
- team-analytics/proto/platform/audit/v1/audit.proto
- team-audit/proto/platform/audit/v1/audit.proto
- team-domain/proto/platform/audit/v1/audit.proto
- team-engagement/proto/platform/audit/v1/audit.proto
- team-frontend/proto/platform/audit/v1/audit.proto
- team-gateway/proto/platform/audit/v1/audit.proto
- team-identity/proto/platform/audit/v1/audit.proto
- team-order/proto/platform/audit/v1/audit.proto
- team-payment/proto/platform/audit/v1/audit.proto
- team-promotion/proto/platform/audit/v1/audit.proto
- team-verification/proto/platform/audit/v1/audit.proto
- team-engagement/generated/**
- team-analytics/generated/**
- openspec/changes/archive/2026-10-22-pah-contract/**

## Read-only dependencies
- openspec/changes/archive/2026-10-22-pah-contract/** is frozen by the LOCK.md digest (only task ticks change)
- platform-core/packages/proto/consumer_copies_test.sh (pattern to mirror)
- plans/privacy-audit-hardening/00-spec.md, slices.yaml, LOCK.md

## Verify
(cd platform-core/packages/proto && buf lint && buf build && bash pah_contract_test.sh && go test ./platform/engagement/v1/... ./platform/audit/v1/...); make -C platform-core lint-proto breaking; for c in team-engagement/proto/platform/engagement/v1/events.proto team-analytics/proto/platform/engagement/v1/events.proto team-order/proto/platform/engagement/v1/events.proto team-analytics/proto/platform/audit/v1/audit.proto team-audit/proto/platform/audit/v1/audit.proto team-domain/proto/platform/audit/v1/audit.proto team-engagement/proto/platform/audit/v1/audit.proto team-frontend/proto/platform/audit/v1/audit.proto team-gateway/proto/platform/audit/v1/audit.proto team-identity/proto/platform/audit/v1/audit.proto team-order/proto/platform/audit/v1/audit.proto team-payment/proto/platform/audit/v1/audit.proto team-promotion/proto/platform/audit/v1/audit.proto team-verification/proto/platform/audit/v1/audit.proto; do s=platform-core/packages/proto/${c#*/proto/}; cmp "$s" "$c" || exit 1; done; for r in team-analytics team-audit team-domain team-engagement team-frontend team-gateway team-identity team-order team-payment team-promotion team-verification; do [ -f $r/proto/buf.yaml ] && (cd $r/proto && buf lint && buf build) || true; done; (cd team-engagement && make proto && go build ./...); (cd team-analytics && make proto && go build ./...); $PY platform-e2e/scripts/spec_sync.py pah-contract --strict  (e2e: none, pure contract; FEATURES not-testable)
