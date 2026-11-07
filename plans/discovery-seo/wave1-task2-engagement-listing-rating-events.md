# [W1-T2] engagement-listing-rating-events

## Role
feature-run worker (change `openspec/changes/engagement-listing-rating-events`)

## Objective
Implement OpenSpec change `engagement-listing-rating-events` test-first per its tasks.md; context in plans/discovery-seo/00-spec.md (C3, C5) and the change's design.md.
- `CreateReview` and `HideReview` enqueue `ListingRatingChanged` (absolute avg + count, principal-less envelope) in the same transaction, only when a row changed, under a per-listing advisory lock; migration 0015 backfills one envelope per reviewed listing.
- depends_on: discovery-seo-contract (merged)
- base: `feat/discovery-seo` after wave 0 merged

## Write-set (EXCLUSIVE)
- team-engagement/internal/store/**
- team-engagement/internal/rpc/**
- team-engagement/internal/events/**
- team-engagement/internal/engagement/**
- team-engagement/migrations/0015_listing_rating_backfill.up.sql
- team-engagement/migrations/0015_listing_rating_backfill.down.sql
- team-engagement/migrations/embed.go
- team-engagement/migrations/migration_test.go
- team-engagement/README.md
- team-engagement/FEATURES.yaml
- platform-e2e/tests/e2e/features/engagement/listing_rating_events.feature
- platform-e2e/tests/e2e/step_definitions/listing_rating_events_steps.py
- platform-e2e/tests/e2e/step_definitions/test_listing_rating_events.py

## Verify
- `cd team-engagement && buf generate && go vet ./... && go test ./...`
- e2e: `cd platform-e2e && HEADLESS=true $PY -m pytest tests/e2e/step_definitions/test_listing_rating_events.py -q` (PY=/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python)
