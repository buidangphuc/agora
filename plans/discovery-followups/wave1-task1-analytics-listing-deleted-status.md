# [W1-T1] analytics-listing-deleted-status

## Role
feature-run worker (change `openspec/changes/analytics-listing-deleted-status`)

## Objective
Implement OpenSpec change `analytics-listing-deleted-status` test-first per its tasks.md; context in plans/discovery-followups/00-spec.md.
- team-analytics writes `LISTING_STATUS_DELETED` on CHANGE_TYPE_DELETED (live + listing-replay), DELETED terminal in the DuckDB upsert; featurestore accepts DELETED; recsys writes visible=false.
- depends_on: discovery-followups-contract (merged)
- base: `asdlc/discovery-followups` after wave 0
- hot file: platform-e2e/tests/e2e/flows/item_attributes_support.py (shared with recs-in-stock-filter, wave 2)

## Write-set (EXCLUSIVE)
- team-analytics/internal/events/**
- team-analytics/internal/warehouse/**
- team-analytics/internal/app/listing_replay*.go
- team-analytics/proto/**
- team-analytics/README.md
- team-analytics/FEATURES.yaml
- platform-featurestore/featurestore/inputs.py
- platform-featurestore/tests/test_item_attributes.py
- platform-featurestore/tests/test_materialize.py
- platform-featurestore/README.md
- platform-featurestore/FEATURES.yaml
- platform-recsys/recsys/load/attributes.py
- platform-recsys/recsys/backfill.py
- platform-recsys/tests/test_candidate_attributes.py
- platform-recsys/tests/test_backfill*.py
- platform-recsys/README.md
- platform-recsys/FEATURES.yaml
- platform-e2e/tests/e2e/features/featurestore/item_attributes.feature
- platform-e2e/tests/e2e/features/recommendations/candidate_attributes.feature
- platform-e2e/tests/e2e/features/recsys/candidate_attributes.feature
- platform-e2e/tests/e2e/step_definitions/item_attributes_steps.py
- platform-e2e/tests/e2e/step_definitions/test_item_attributes.py
- platform-e2e/tests/e2e/step_definitions/candidate_attributes_steps.py
- platform-e2e/tests/e2e/step_definitions/test_candidate_attributes.py
- platform-e2e/tests/e2e/flows/item_attributes_support.py
- platform-e2e/tests/e2e/step_definitions/fia_steps.py
- platform-e2e/tests/e2e/flows/fia_flow.py
- platform-e2e/tests/e2e/flows/fia_job_driver.py

## Verify
- `cd team-analytics && DOCKER_HOST=unix:///Users/phuc.buidang/.orbstack/run/docker.sock GO_SDK_REQUIRE_DOCKER=1 go test ./...`
- `cd platform-featurestore && ruff check . && python -m pytest -q tests/`
- `cd platform-recsys && make lint test-host`
- e2e: item_attributes + candidate_attributes features against env discovery-followups
