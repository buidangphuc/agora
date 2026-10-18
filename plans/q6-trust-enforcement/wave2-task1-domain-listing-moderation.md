# [W2-T1] domain-listing-moderation

## Role
feature-run worker (change `openspec/changes/archive/2026-10-18-domain-listing-moderation`)

## Objective
Implement OpenSpec change `domain-listing-moderation` test-first per its tasks.md (scenario ids there); context in plans/q6-trust-enforcement/00-spec.md and the change's design.md Clarifications.
- team-domain owns listing status: X3 quick fix (client status ignored, REJECTED sticky), keyword + AI auto-screen on publish/content edit, PENDING_REVIEW/BANNED/HELD, ModerateListing (audit-first) and ListModerationQueue; gateway admin routes.
- depends_on: ai-listing-screen

## Write-set (EXCLUSIVE)
- team-domain/internal/service/listing.go
- team-domain/internal/service/listing_test.go
- team-domain/internal/service/moderation.go
- team-domain/internal/service/moderation_test.go
- team-domain/internal/handler/listing.go
- team-domain/internal/handler/listing_test.go
- team-domain/internal/handler/listing_visibility_test.go (widened at build: task 1) # widened at build: task 1
- team-domain/internal/handler/listing_moderation_view_test.go (widened at build: task 2) # widened at build: task 2
- team-domain/internal/handler/listing_audit_test.go (widened at build: task 7) # widened at build: task 7
- team-domain/internal/handler/moderation.go
- team-domain/internal/handler/moderation_test.go
- team-domain/proto/platform/listing/v1/*.proto
- team-domain/proto/platform/ai/v1/*.proto
- team-domain/generated/**
- team-domain/migrations/0011_listing_moderation.up.sql
- team-domain/migrations/0011_listing_moderation.down.sql
- team-domain/internal/repository/listing.go
- team-domain/internal/repository/listing_pg.go
- team-domain/internal/repository/outbox_pg.go
- team-domain/internal/repository/*_test.go
- team-domain/internal/screen/**
- team-domain/internal/config/config.go
- team-domain/internal/config/config_test.go
- team-domain/internal/upstream/ai.go
- team-domain/internal/upstream/ai_test.go
- team-domain/internal/upstream/audit.go
- team-domain/internal/upstream/audit_test.go
- team-domain/internal/bootstrap/resources.go
- team-domain/cmd/server/main.go
- team-domain/docker-compose.local.yaml
- team-domain/.env.example
- team-domain/README.md
- team-domain/go.mod (widened at build: task 4) # widened at build: task 4
- team-domain/FEATURES.yaml
- team-gateway/internal/edge/policy.go
- team-gateway/internal/edge/listing.go
- team-gateway/internal/edge/policy_edge_test.go
- team-gateway/internal/edge/listing_test.go
- team-gateway/internal/edge/unrouted_test.go
- team-gateway/internal/edge/policy_test.go (widened at build: task 9) # widened at build: task 9
- team-gateway/generated/**
- team-gateway/FEATURES.yaml
- platform-e2e/tests/e2e/features/seller/listing_moderation.feature
- platform-e2e/tests/e2e/step_definitions/test_listing_moderation.py
- platform-e2e/tests/e2e/support/listing_moderation_support.py
- platform-e2e/tests/e2e/features/admin/listing_admin_audit.feature
- platform-e2e/tests/e2e/step_definitions/test_listing_admin_audit.py
- docker-compose.services.yaml
- openspec/changes/archive/2026-10-18-domain-listing-moderation/**

## Read-only dependencies
- openspec/changes/archive/2026-10-18-domain-listing-moderation/** is frozen by the LOCK.md digest (only task ticks change)
- platform-e2e/tests/e2e/support/audit_support.py (reuse, do not edit)
- team-search/internal/taxonomy/classifier.go (principal pattern)
- plans/q6-trust-enforcement/00-spec.md, slices.yaml, LOCK.md

## Verify
(cd team-domain && make proto && go vet ./... && go test ./... -race); (cd team-gateway && go vet ./... && go test ./internal/edge/... -race); (cd platform-e2e && HEADLESS=true $PY -m pytest -q tests/e2e/step_definitions/test_listing_moderation.py tests/e2e/step_definitions/test_listing_admin_audit.py -m 'not destructive') then the @destructive audit-down scenario alone; $PY platform-e2e/scripts/spec_sync.py domain-listing-moderation --strict
