# [W5-T1] ai-copilot-grounding

## Role
feature-run worker (change `openspec/changes/ai-copilot-grounding`)

## Objective
Implement OpenSpec change `ai-copilot-grounding` test-first per its tasks.md; context in plans/docs-ai-data/00-spec.md.
- ChatCopilot grounded on the seller's own published listing via the `ListingLookup` port (200 ms), no delivery-time promises, `COPILOT_BACKEND` mock|llm_router with fact validator and fallback; provenance (`request_id`, `model_version`) on ChatCopilot, MagicListing, SummarizeReviews; copilot eval set.
- depends_on: docs-ai-data-contract, ai-assistant-grounding (merged)
- base: `asdlc/docs-ai-data` after wave 4
- MINOR to fold in: scenarios for the 1000/3/300-character limits, the variant-sum stock rule and the LLM delivery-claim fallback; move "Default drafts carry no availability claim" under stock grounding.

## Write-set (EXCLUSIVE)
- team-ai/app/**
- team-ai/tests/**
- team-ai/evals/**
- team-ai/scripts/run_eval.py
- team-ai/.env.example
- team-ai/README.md
- team-ai/FEATURES.yaml
- platform-e2e/tests/e2e/features/ai/chat_copilot.feature
- platform-e2e/tests/e2e/features/ai/magic_listing.feature
- platform-e2e/tests/e2e/step_definitions/test_chat_copilot.py
- platform-e2e/tests/e2e/step_definitions/test_magic_listing.py
- platform-e2e/tests/e2e/step_definitions/magic_listing_steps.py
- platform-e2e/tests/e2e/step_definitions/chat_copilot_steps.py

## Verify
- `cd team-ai && make ci && make eval`
- `openspec validate ai-copilot-grounding --strict`
- `make -C platform-e2e features-check`
- e2e: ai/chat_copilot.feature, ai/magic_listing.feature (team-ai, team-domain, team-gateway + deps)
