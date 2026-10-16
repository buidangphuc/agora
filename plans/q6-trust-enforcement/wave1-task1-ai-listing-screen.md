# [W1-T1] ai-listing-screen

## Role
feature-run worker (change `openspec/changes/ai-listing-screen`)

## Objective
Implement OpenSpec change `ai-listing-screen` test-first per its tasks.md (scenario ids there); context in plans/q6-trust-enforcement/00-spec.md and the change's design.md Clarifications.
- team-ai ScreenListing (internal, service principal + ai.classify scope): mock backend default (marker substring), llm_router behind team-ai with timeout -> UNAVAILABLE; listing indexer indexes only PUBLISHED.
- depends_on: q6-trust-contract

## Write-set (EXCLUSIVE)
- team-ai/app/core/config/ai.py
- team-ai/app/core/**
- team-ai/.env.example
- team-ai/tests/unit/core/test_screen_settings.py
- team-ai/app/modules/business/listing_screen/**
- team-ai/tests/unit/modules/screen/**
- team-ai/app/transport/grpc/scopes.py
- team-ai/app/transport/grpc/servicers/ai.py
- team-ai/tests/unit/transport/test_ai_servicer.py
- team-ai/tests/unit/transport/test_grpc_scopes.py
- team-ai/app/modules/messaging/listing_indexer/decode.py
- team-ai/app/modules/messaging/listing_indexer/handler.py
- team-ai/tests/unit/modules/listing_indexer/test_decode.py
- team-ai/tests/unit/modules/listing_indexer/test_handler.py
- team-ai/generated/**
- team-ai/README.md
- team-ai/FEATURES.yaml
- platform-e2e/tests/e2e/features/ai/listing_screen_edge.feature
- platform-e2e/tests/e2e/step_definitions/test_listing_screen_edge.py
- openspec/changes/ai-listing-screen/**
- team-ai/tests/unit/test_readme_screen_listing.py (# widened at build: task 5)
- plans/q6-trust-enforcement/wave1-task1-ai-listing-screen.md (# widened at build: task 5; packet widening record)

## Read-only dependencies
- openspec/changes/ai-listing-screen/** is frozen by the LOCK.md digest (only task ticks change)
- team-ai/proto/platform/{ai,listing}/v1/*.proto (copied by q6-trust-contract)
- platform-e2e mls_steps.classify_at_edge helpers (reuse)
- plans/q6-trust-enforcement/00-spec.md, slices.yaml, LOCK.md

## Verify
(cd team-ai && make proto && make lint typecheck test); (cd platform-e2e && HEADLESS=true $PY -m pytest -q tests/e2e/step_definitions/test_listing_screen_edge.py); $PY platform-e2e/scripts/spec_sync.py ai-listing-screen --strict
