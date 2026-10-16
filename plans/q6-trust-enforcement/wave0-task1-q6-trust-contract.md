# [W0-T1] q6-trust-contract

## Role
feature-run worker (change `openspec/changes/q6-trust-contract`)

## Objective
Implement OpenSpec change `q6-trust-contract` test-first per its tasks.md (scenario ids there); context in plans/q6-trust-enforcement/00-spec.md and the change's design.md Clarifications.
- Additive trust contract: listing moderation statuses/RPCs, AIService.ScreenListing, AccountAdminService + UserStatusChanged, report RPCs, dispute ruling/lists + DisputeResolved, audit catalogue to 17 actions; byte-identical copies into the consumer repos.
- depends_on: none

## Write-set (EXCLUSIVE)
- platform-core/packages/proto/platform/listing/v1/*.proto
- platform-core/packages/proto/platform/ai/v1/*.proto
- platform-core/packages/proto/platform/identity/v1/*.proto
- platform-core/packages/proto/platform/engagement/v1/*.proto
- platform-core/packages/proto/platform/audit/v1/*.proto
- platform-core/packages/proto/platform/listing/v1/*_test.go
- platform-core/packages/proto/platform/ai/v1/*_test.go
- platform-core/packages/proto/platform/identity/v1/*_test.go
- platform-core/packages/proto/platform/engagement/v1/*_test.go
- platform-core/packages/proto/platform/audit/v1/*_test.go
- platform-core/gen/**
- platform-core/docs/ROADMAP.md
- platform-core/FEATURES.yaml
- team-domain/proto/platform/listing/v1/listing.proto
- team-gateway/proto/platform/listing/v1/listing.proto
- team-frontend/proto/platform/listing/v1/listing.proto
- team-ai/proto/platform/listing/v1/listing.proto
- team-search/proto/platform/listing/v1/listing.proto
- team-order/proto/platform/listing/v1/listing.proto
- team-ai/proto/platform/ai/v1/ai.proto
- team-domain/proto/platform/ai/v1/ai.proto
- team-gateway/proto/platform/ai/v1/ai.proto
- team-frontend/proto/platform/ai/v1/ai.proto
- team-search/proto/platform/ai/v1/ai.proto
- team-analytics/proto/platform/ai/v1/ai.proto
- team-identity/proto/platform/identity/v1/identity.proto
- team-gateway/proto/platform/identity/v1/identity.proto
- team-frontend/proto/platform/identity/v1/identity.proto
- team-domain/proto/platform/identity/v1/identity.proto
- team-engagement/proto/platform/engagement/v1/engagement.proto
- team-gateway/proto/platform/engagement/v1/engagement.proto
- team-frontend/proto/platform/engagement/v1/engagement.proto
- team-order/proto/platform/engagement/v1/engagement.proto
- team-analytics/proto/platform/engagement/v1/engagement.proto
- team-engagement/proto/platform/engagement/v1/events.proto
- team-analytics/proto/platform/engagement/v1/events.proto
- team-order/proto/platform/engagement/v1/events.proto
- team-analytics/proto/platform/audit/v1/audit.proto
- team-audit/proto/platform/audit/v1/audit.proto
- team-domain/proto/platform/audit/v1/audit.proto
- team-engagement/proto/platform/audit/v1/audit.proto
- team-frontend/proto/platform/audit/v1/audit.proto
- team-gateway/proto/platform/audit/v1/audit.proto
- team-order/proto/platform/audit/v1/audit.proto
- team-payment/proto/platform/audit/v1/audit.proto
- team-promotion/proto/platform/audit/v1/audit.proto
- team-verification/proto/platform/audit/v1/audit.proto
- openspec/changes/q6-trust-contract/**
- team-ai/tests/unit/transport/test_grpc_scopes.py (# widened at build: task 1.6+2.3)
- team-ai/tests/unit/transport/test_proto_stubs.py (# widened at build: task 1.6+2.3)
- plans/q6-trust-enforcement/wave0-task1-q6-trust-contract.md (# widened at build: task 1.6+2.3; packet widening record)

## Read-only dependencies
- openspec/changes/q6-trust-contract/** is frozen by the LOCK.md digest (only task ticks change)
- platform-core/packages/proto/platform/promotion/v1/flashstock_compat_test.go (pattern to mirror)
- plans/q6-trust-enforcement/00-spec.md, slices.yaml, LOCK.md

## Verify
make -C platform-core lint-proto breaking; (cd platform-core/packages/proto && buf lint && buf build && go test ./platform/listing/v1/... ./platform/ai/v1/... ./platform/identity/v1/... ./platform/engagement/v1/... ./platform/audit/v1/...); make -C platform-core proto; for c in team-domain/proto/platform/listing/v1/listing.proto team-gateway/proto/platform/listing/v1/listing.proto team-frontend/proto/platform/listing/v1/listing.proto team-ai/proto/platform/listing/v1/listing.proto team-search/proto/platform/listing/v1/listing.proto team-order/proto/platform/listing/v1/listing.proto team-ai/proto/platform/ai/v1/ai.proto team-domain/proto/platform/ai/v1/ai.proto team-gateway/proto/platform/ai/v1/ai.proto team-frontend/proto/platform/ai/v1/ai.proto team-search/proto/platform/ai/v1/ai.proto team-analytics/proto/platform/ai/v1/ai.proto team-identity/proto/platform/identity/v1/identity.proto team-gateway/proto/platform/identity/v1/identity.proto team-frontend/proto/platform/identity/v1/identity.proto team-domain/proto/platform/identity/v1/identity.proto team-engagement/proto/platform/engagement/v1/engagement.proto team-gateway/proto/platform/engagement/v1/engagement.proto team-frontend/proto/platform/engagement/v1/engagement.proto team-order/proto/platform/engagement/v1/engagement.proto team-analytics/proto/platform/engagement/v1/engagement.proto team-engagement/proto/platform/engagement/v1/events.proto team-analytics/proto/platform/engagement/v1/events.proto team-order/proto/platform/engagement/v1/events.proto team-analytics/proto/platform/audit/v1/audit.proto team-audit/proto/platform/audit/v1/audit.proto team-domain/proto/platform/audit/v1/audit.proto team-engagement/proto/platform/audit/v1/audit.proto team-frontend/proto/platform/audit/v1/audit.proto team-gateway/proto/platform/audit/v1/audit.proto team-order/proto/platform/audit/v1/audit.proto team-payment/proto/platform/audit/v1/audit.proto team-promotion/proto/platform/audit/v1/audit.proto team-verification/proto/platform/audit/v1/audit.proto; do s=platform-core/packages/proto/${c#*/proto/}; cmp "$s" "$c" || exit 1; done; for r in team-domain team-gateway team-frontend team-ai team-search team-order team-identity team-engagement team-analytics team-audit team-payment team-promotion team-verification; do [ -f $r/proto/buf.yaml ] && (cd $r/proto && buf lint) || true; done; $PY platform-e2e/scripts/spec_sync.py q6-trust-contract --strict  (e2e: none, pure contract; FEATURES not-testable)
