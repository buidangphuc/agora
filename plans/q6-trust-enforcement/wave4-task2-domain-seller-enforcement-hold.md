# [W4-T2] domain-seller-enforcement-hold

## Role
feature-run worker (change `openspec/changes/domain-seller-enforcement-hold`)

## Objective
Implement OpenSpec change `domain-seller-enforcement-hold` test-first per its tasks.md (scenario ids there); context in plans/q6-trust-enforcement/00-spec.md and the change's design.md Clarifications.
- team-domain consumes identity UserStatusChanged: SUSPENDED/BANNED hold a seller's published/pending listings (HELD, prior status kept), ACTIVE/RESTRICTED restore; dedupe + stale-event guard; HELD is unreadable to others, unwritable, undeletable and unreservable.
- depends_on: domain-listing-moderation, identity-user-enforcement

## Write-set (EXCLUSIVE)
- team-domain/migrations/*_seller_enforcement_hold.up.sql
- team-domain/migrations/*_seller_enforcement_hold.down.sql
- team-domain/internal/repository/listing.go
- team-domain/internal/repository/listing_pg.go
- team-domain/internal/repository/seller_hold.go
- team-domain/internal/repository/seller_hold_test.go
- team-domain/internal/repository/*_test.go
- team-domain/internal/consumer/**
- team-domain/internal/config/config.go
- team-domain/internal/config/config_test.go
- team-domain/internal/bootstrap/addon.go
- team-domain/internal/bootstrap/identity_consumer.go
- team-domain/cmd/server/main.go
- team-domain/docker-compose.local.yaml
- team-domain/internal/handler/listing.go
- team-domain/internal/handler/listing_test.go
- team-domain/internal/service/listing.go
- team-domain/internal/service/listing_test.go
- team-domain/internal/grpcserver/server_test.go
- team-domain/generated/**
- team-domain/.env.example
- team-domain/README.md
- team-domain/FEATURES.yaml
- platform-e2e/tests/e2e/features/seller/seller_enforcement_hold.feature
- platform-e2e/tests/e2e/step_definitions/test_seller_enforcement_hold.py
- openspec/changes/domain-seller-enforcement-hold/**

## Read-only dependencies
- openspec/changes/domain-seller-enforcement-hold/** is frozen by the LOCK.md digest (only task ticks change)
- team-domain/proto/platform/identity/v1/identity.proto (copied by q6-trust-contract)
- platform-e2e/tests/e2e/support/audit_support.py, user_enforcement_support.py (reuse)
- team-search/internal/consumer/consumer.go (pattern)
- plans/q6-trust-enforcement/00-spec.md, slices.yaml, LOCK.md

## Verify
(cd team-domain && make proto && go vet ./... && go test ./... -race); (cd platform-e2e && HEADLESS=true $PY -m pytest -q tests/e2e/step_definitions/test_seller_enforcement_hold.py); $PY platform-e2e/scripts/spec_sync.py domain-seller-enforcement-hold --strict
