# [W1-T3] listing-plan-cap

## Role
feature-run worker (change `openspec/changes/listing-plan-cap`)

## Objective
Implement OpenSpec change `listing-plan-cap` test-first per its tasks.md; context in plans/monetization/00-spec.md (decisions in LOCK.md, defaults in brackets apply).
- team-domain enforces `max_listings` on create (per-seller advisory lock, count all statuses) and `featured_listings` on `UpsertStorefront`, via `SubscriptionService.GetEntitlements` with the service principal, 2 s timeout, fail closed (UNAVAILABLE).
- RESOURCE_EXHAUSTED with `ErrorInfo{Reason: PLAN_LISTING_LIMIT}` / `PLAN_FEATURED_LIMIT`; required `UPSTREAM_PROMOTION_ADDR`.
- depends_on: monetization-contract
- base: `asdlc/monetization` after wave 0
- hot file: docker-compose.services.yaml (team-domain UPSTREAM_PROMOTION_ADDR) shared with promotion-plan-billing (wave 3)

## Write-set (EXCLUSIVE)
- team-domain/internal/config/**
- team-domain/internal/listing/**
- team-domain/internal/store/**
- team-domain/internal/rpc/**
- team-domain/internal/app/**
- team-domain/go.mod
- team-domain/go.sum
- team-domain/README.md
- team-domain/.env.example
- team-domain/FEATURES.yaml
- docker-compose.services.yaml
- docker-compose.yaml
- platform-e2e/tests/e2e/features/listing/listing_plan_cap.feature
- platform-e2e/tests/e2e/step_definitions/listing_plan_cap_steps.py
- platform-e2e/tests/e2e/support/listing_plan_cap_support.py
- platform-e2e/tests/e2e/step_definitions/test_listing_plan_cap.py

## Verify
- `DOCKER_HOST=unix:///Users/phuc.buidang/.orbstack/run/docker.sock GO_SDK_REQUIRE_DOCKER=1 /Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py go team-domain` (equivalent: `cd team-domain && DOCKER_HOST=unix:///Users/phuc.buidang/.orbstack/run/docker.sock GO_SDK_REQUIRE_DOCKER=1 go test ./...`)
- `cd team-domain && make check`
- e2e: listing/listing_plan_cap.feature against env monetization (team-domain, team-promotion, team-gateway)
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py spec-sync . listing-plan-cap`
