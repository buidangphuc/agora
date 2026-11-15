# [W2-T3] buyer-sponsored-slots

## Role
feature-run worker (change `openspec/changes/buyer-sponsored-slots`)

## Objective
Implement OpenSpec change `buyer-sponsored-slots` test-first per its tasks.md; context in plans/monetization/00-spec.md (decisions in LOCK.md, defaults in brackets apply).
- team-frontend renders at most two labelled "Tài trợ" slots on search page 1 (category) and the home feed, 150 ms race, dedupe against organic results, click beacon through `POST /api/ads/click` (204 always, no ids logged).
- depends_on: promotion-ads-serving
- base: `asdlc/monetization` after wave 1
- hot file: none (team-frontend only in this wave)

## Write-set (EXCLUSIVE)
- team-frontend/src/lib/gateway/promotion.ts
- team-frontend/src/lib/gateway/promotion.test.ts
- team-frontend/src/features/ads/**
- team-frontend/src/app/api/ads/**
- team-frontend/src/features/search/SearchBlocks.tsx
- team-frontend/src/features/search/SearchBlocks.test.tsx
- team-frontend/src/features/home/FeedBlock.tsx
- team-frontend/src/features/home/blocks.test.tsx
- team-frontend/README.md
- team-frontend/FEATURES.yaml
- platform-e2e/tests/e2e/features/frontend/buyer_sponsored_slots.feature
- platform-e2e/tests/e2e/step_definitions/buyer_sponsored_slots_steps.py
- platform-e2e/tests/e2e/support/buyer_sponsored_slots_support.py
- platform-e2e/tests/e2e/step_definitions/test_buyer_sponsored_slots.py
- widened (P1):
  - openspec/changes/buyer-sponsored-slots/tasks.md — this change's own OpenSpec folder (task ticks)

## Verify
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py vitest team-frontend`
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py tsc team-frontend`
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py biome team-frontend`
- `cd team-frontend && npm run check`
- e2e: frontend/buyer_sponsored_slots.feature against env monetization (team-frontend, team-gateway, team-promotion, team-domain)
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py spec-sync . buyer-sponsored-slots`
