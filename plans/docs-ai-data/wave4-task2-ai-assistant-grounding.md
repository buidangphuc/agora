# [W4-T2] ai-assistant-grounding

## Role
feature-run worker (change `openspec/changes/ai-assistant-grounding`)

## Objective
Implement OpenSpec change `ai-assistant-grounding` test-first per its tasks.md; context in plans/docs-ai-data/00-spec.md.
- ShoppingAssistant grounded on SearchListings (`in_stock=true`, budget, 5) + GetListing (PUBLISHED only), mock/LLM replier with post-check and template fallback, `request_id` + `model_version` (`mock:assistant-v1|<search mv>`), delete demo catalog and RAG wiring, eval set >= 0.8; compose env `ASSISTANT_LISTING_ADDR`, `ASSISTANT_BACKEND=mock`.
- depends_on: docs-ai-data-contract, recs-experiment-arms (merged)
- base: `asdlc/docs-ai-data` after wave 3
- hot files: team-ai/app/core/config/ai.py, app/bootstrap/services.py, app/transport/grpc/servicers/ai.py (ai-copilot-grounding W5); docker-compose.services.yaml

## Write-set (EXCLUSIVE)
- team-ai/app/**
- team-ai/tests/**
- team-ai/evals/**
- team-ai/scripts/run_eval.py
- team-ai/.env.example
- team-ai/README.md
- team-ai/FEATURES.yaml
- docker-compose.services.yaml
- platform-e2e/tests/e2e/features/ai/assistant_grounding.feature
- platform-e2e/tests/e2e/features/ai/assistant.feature
- platform-e2e/tests/e2e/step_definitions/test_assistant_grounding.py
- platform-e2e/tests/e2e/step_definitions/test_assistant.py
- platform-e2e/tests/e2e/step_definitions/assistant_steps.py

## Verify
- `cd team-ai && make ci && make eval`
- `openspec validate ai-assistant-grounding --strict`
- `make -C platform-e2e features-check`
- e2e: ai/assistant_grounding.feature, ai/assistant.feature (search-down scenario is destructive, restores team-search)
