# [W3-T1] frontend-ai-interaction-events

## Role
feature-run worker (change `openspec/changes/frontend-ai-interaction-events`)

## Objective
Implement OpenSpec change `frontend-ai-interaction-events` test-first per its tasks.md (scenario ids there); context in plans/q4-loop-instrumentation/00-spec.md and the change's design.md Clarifications.
- ai_interaction {surface, action} from /assistant, seller chat copilot, Magic Listing and the PDP review summary; mock team-ai only.

## Write-set (EXCLUSIVE)
- team-frontend/src/lib/analytics/ai.ts
- team-frontend/src/lib/analytics/ai.test.ts
- team-frontend/src/features/assistant/AssistantChatView.tsx
- team-frontend/src/features/assistant/AssistantChatView.test.tsx
- team-frontend/src/features/chat/ChatView.tsx
- team-frontend/src/features/chat/ChatView.test.tsx
- team-frontend/src/features/listing/MagicListingAssist.tsx
- team-frontend/src/features/listing/MagicListingAssist.test.tsx
- team-frontend/src/features/listing/ListingForm.tsx
- team-frontend/src/features/listing/ListingForm.test.tsx
- team-frontend/src/features/review/AiReviewSummary.tsx
- team-frontend/src/features/review/AiReviewSummaryShown.tsx
- team-frontend/src/features/review/AiReviewSummary.test.tsx
- team-frontend/FEATURES.yaml
- platform-e2e/tests/e2e/support/ai_events_support.py
- platform-e2e/tests/e2e/features/tracking/ai_interaction_events.feature
- platform-e2e/tests/e2e/step_definitions/ai_interaction_steps.py
- platform-e2e/tests/e2e/step_definitions/test_ai_interaction_events.py

## Read-only dependencies
- openspec/changes/frontend-ai-interaction-events/** (frozen by LOCK.md digest)
- team-frontend/src/lib/analytics/schema.ts, map.ts (landed by frontend-loop-events; read-only here)
- platform-e2e/tests/e2e/support/loop_events_support.py, review_order_support.py (import, never edit)
- plans/q4-loop-instrumentation/00-spec.md, slices.yaml, LOCK.md

## Verify
(cd team-frontend && npx vitest run && npx tsc --noEmit && npx prettier --check src); (cd platform-e2e && HEADLESS=true $PY -m pytest -q tests/e2e/step_definitions/test_ai_interaction_events.py); $PY platform-e2e/scripts/spec_sync.py frontend-ai-interaction-events --strict
