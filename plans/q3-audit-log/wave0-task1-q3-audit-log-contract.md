# [W0-T1] q3-audit-log-contract

## Role
feature-run worker (change `openspec/changes/archive/2026-10-18-q3-audit-log-contract`)

## Objective
Implement OpenSpec change `q3-audit-log-contract` test-first per its tasks.md (scenario ids there); context in plans/q3-audit-log/00-spec.md and the change's design.md Clarifications.
- Also vendors the updated audit.proto byte-identical into the ten consumer trees (four updated, six new); consumers regenerate their own gitignored code.
- Owns the root compose `UPSTREAM_AUDIT_ADDR` entries for the five new producers, the AGENTS.md rule and the auth-scope-reviewer check.

## Write-set (EXCLUSIVE)
- platform-core/packages/proto/platform/audit/v1/audit.proto
- platform-core/packages/proto/platform/audit/v1/auditquery_roundtrip_test.go
- platform-core/FEATURES.yaml
- team-audit/proto/platform/audit/v1/audit.proto
- team-gateway/proto/platform/audit/v1/audit.proto
- team-analytics/proto/platform/audit/v1/audit.proto
- team-engagement/proto/platform/audit/v1/audit.proto
- team-frontend/proto/platform/audit/v1/audit.proto
- team-verification/proto/platform/audit/v1/audit.proto
- team-domain/proto/platform/audit/v1/audit.proto
- team-order/proto/platform/audit/v1/audit.proto
- team-payment/proto/platform/audit/v1/audit.proto
- team-promotion/proto/platform/audit/v1/audit.proto
- docker-compose.services.yaml
- AGENTS.md
- .claude/agents/auth-scope-reviewer.md

## Read-only dependencies
- openspec/changes/archive/2026-10-18-q3-audit-log-contract/** (frozen by LOCK.md digest)
- platform-core/packages/proto/platform/engagement/v1/hidereview_roundtrip_test.go (pattern to mirror)
- plans/q3-audit-log/00-spec.md, plans/q3-audit-log/slices.yaml, plans/q3-audit-log/LOCK.md

## Verify
make -C platform-core lint-proto breaking; (cd platform-core/packages/proto && buf lint && buf build && buf generate && go test ./platform/audit/v1/...); for r in team-audit team-gateway team-analytics team-engagement team-frontend team-verification team-domain team-order team-payment team-promotion; do cmp platform-core/packages/proto/platform/audit/v1/audit.proto $r/proto/platform/audit/v1/audit.proto || exit 1; [ -f $r/proto/buf.yaml ] && (cd $r/proto && buf lint); done; docker compose -f docker-compose.services.yaml config -q; platform-e2e/.venv/bin/python platform-e2e/scripts/spec_sync.py q3-audit-log-contract --strict
