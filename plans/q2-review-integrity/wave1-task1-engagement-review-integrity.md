# [W1-T1] engagement-review-integrity

## Role
feature-run worker (change `openspec/changes/engagement-review-integrity`)

## Objective
Implement OpenSpec change `engagement-review-integrity` test-first per its tasks.md (scenario ids there); context in plans/q2-review-integrity/00-spec.md and the change's design.md Clarifications. Includes the team-gateway forwarder + admin policy, the shared e2e fixture, migration of API-level review e2e, platform-core seed tools and the compose env.

## Write-set (EXCLUSIVE)
- team-engagement/migrations/0011_review_integrity.up.sql
- team-engagement/migrations/0011_review_integrity.down.sql
- team-engagement/internal/repository/**
- team-engagement/internal/upstream/**
- team-engagement/internal/config/**
- team-engagement/internal/bootstrap/**
- team-engagement/internal/service/review.go
- team-engagement/internal/service/review_test.go
- team-engagement/internal/handler/**
- team-engagement/cmd/**
- team-engagement/internal/grpcserver/server_test.go
- team-engagement/docker-compose.local.yaml
- team-engagement/.env.example
- team-engagement/README.md
- team-engagement/FEATURES.yaml
- team-engagement/proto/platform/audit/v1/audit.proto
- team-gateway/internal/edge/engagement.go
- team-gateway/internal/edge/engagement*_test.go
- team-gateway/internal/edge/policy.go
- team-gateway/internal/edge/policy_test.go
- team-gateway/internal/edge/policy_edge_test.go
- team-gateway/internal/edge/policy_edge_hidereview_test.go
- team-gateway/internal/edge/unrouted_test.go
- team-gateway/FEATURES.yaml
- platform-e2e/tests/e2e/support/review_order_support.py
- platform-e2e/tests/e2e/support/efe_support.py
- platform-e2e/src/api/services/engagement_service.py
- platform-e2e/tests/e2e/features/engagement/**
- platform-e2e/tests/e2e/features/buyer/review_ratings_filter.feature
- platform-e2e/tests/e2e/step_definitions/review_ratings_steps.py
- platform-e2e/tests/e2e/step_definitions/test_rich_reviews.py
- platform-e2e/tests/e2e/step_definitions/rvp_steps.py
- platform-e2e/tests/e2e/step_definitions/test_engagement.py
- platform-e2e/tests/e2e/step_definitions/test_review_ratings_filter.py
- platform-e2e/tests/e2e/step_definitions/pdp_reviews_steps.py
- platform-e2e/tests/e2e/step_definitions/efe_steps.py
- platform-e2e/tests/e2e/step_definitions/test_review_integrity.py
- platform-e2e/tests/e2e/step_definitions/group_c_steps.py
- platform-e2e/tests/e2e/step_definitions/test_reviews.py
- platform-core/tools/seed_full_marketplace.py
- platform-core/tools/seed/01-base-info.sh
- platform-core/tools/seed-full-marketplace.sh
- platform-core/tools/run_live_user_journeys.py
- docker-compose.services.yaml

## Read-only dependencies
- openspec/changes/engagement-review-integrity/** (frozen by LOCK.md digest)
- q2-review-integrity-contract output (proto copies in team-engagement/proto, team-gateway/proto)
- plans/q2-review-integrity/00-spec.md, slices.yaml, LOCK.md

## Verify
(cd team-engagement && go test ./...); (cd team-gateway && go test ./internal/edge/...); (cd platform-e2e && HEADLESS=true .venv/bin/python -m pytest -q tests/e2e/features/engagement tests/e2e/features/buyer/review_ratings_filter.feature); platform-e2e/.venv/bin/python platform-e2e/scripts/spec_sync.py engagement-review-integrity --strict
