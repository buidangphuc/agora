# [W6-T2] buyer-ai-surfaces-ui

## Role
feature-run worker (change `openspec/changes/buyer-ai-surfaces-ui`)

## Objective
Implement OpenSpec change `buyer-ai-surfaces-ui` test-first per its tasks.md; context in plans/docs-ai-data/00-spec.md.
- /assistant is unary (delete the stream route), cards tracked with `placementId=assistant.suggestions` and provenance in wire fields only; copilot/review beacons carry provenance; home AssistantBlock is a plain link; server-only `getVariant` (50 ms) + `ExperimentExposure` once per mount.
- depends_on: ai-assistant-grounding, ai-copilot-grounding, analytics-experiment-reports (merged)
- base: `asdlc/docs-ai-data` after wave 5
- Generated stubs are gitignored: `npm ci && npm run proto` in the worktree.
- MINOR to fold in: design.md:9 points at a missing "open issue 1"; add an Open issues entry.

## Write-set (EXCLUSIVE)
- team-frontend/src/**
- team-frontend/.env.example
- team-frontend/README.md
- team-frontend/FEATURES.yaml
- platform-e2e/tests/e2e/features/frontend/buyer_ai_surfaces.feature
- platform-e2e/tests/e2e/features/tracking/ai_interaction_events.feature
- platform-e2e/tests/e2e/step_definitions/test_buyer_ai_surfaces.py
- platform-e2e/tests/e2e/step_definitions/buyer_ai_surfaces_steps.py
- platform-e2e/tests/e2e/step_definitions/test_ai_interaction_events.py
- platform-e2e/tests/e2e/step_definitions/ai_interaction_steps.py
- platform-e2e/tests/e2e/step_definitions/frontend_steps.py

## Verify
- `cd team-frontend && npm ci && npm run proto && npm run check`
- `openspec validate buyer-ai-surfaces-ui --strict`
- `make -C platform-e2e features-check`
- e2e: frontend/buyer_ai_surfaces.feature, tracking/ai_interaction_events.feature (mainx, mock AI backend)
