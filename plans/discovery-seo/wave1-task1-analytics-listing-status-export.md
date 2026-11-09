# [W1-T1] analytics-listing-status-export

## Role
feature-run worker (change `openspec/changes/analytics-listing-status-export`)

## Objective
Implement OpenSpec change `analytics-listing-status-export` test-first per its tasks.md; context in plans/discovery-seo/00-spec.md and the change's design.md.
- `listing_sellers` gains a nullable `status` column (enum name, null for deleted/unspecified/unknown) written under the existing `>=` guard and exported last in `listing_sellers.parquet`; one-off `cmd/listing-replay` with its own consumer group (`KAFKA_LISTING_REPLAY_GROUP`) backfills existing rows.
- depends_on: none (no proto change; uses the existing listing.v1 enum)
- base: `feat/discovery-seo` after wave 0 merged

## Write-set (EXCLUSIVE)
- team-analytics/internal/events/**
- team-analytics/internal/warehouse/**
- team-analytics/internal/export/**
- team-analytics/internal/app/**
- team-analytics/internal/config/**
- team-analytics/cmd/listing-replay/**
- team-analytics/Dockerfile
- team-analytics/Makefile
- team-analytics/README.md
- team-analytics/.env.example
- team-analytics/FEATURES.yaml
- platform-e2e/tests/e2e/features/featurestore/item_attributes.feature
- platform-e2e/tests/e2e/step_definitions/fia_steps.py
- platform-e2e/tests/e2e/step_definitions/test_fia_item_attributes.py

## Verify
- `cd team-analytics && buf generate --template buf.gen.local.yaml && go vet ./... && go test ./...`
- e2e: `cd platform-e2e && HEADLESS=true $PY -m pytest tests/e2e/step_definitions/test_fia_item_attributes.py -q` (PY=/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python; services: team-analytics, redpanda, listing create/ban path only)
