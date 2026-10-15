# LOCK — q4-loop-instrumentation
status: approved
approved-by: Bùi Đăng Phúc (chat "ok", 2026-10-15)
date: 2026-10-15
spec: plans/q4-loop-instrumentation/00-spec.md (Locked 2026-10-15)
## Slices
| slice | owner | depends_on | rounds | validate | spec_lint | spec-quality | decide (spec) | digest | approve |
|---|---|---|---|---|---|---|---|---|---|
| q4-loop-events-contract | platform-core | [] | 2 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | 3ec4a3ad52ef | [x] |
| analytics-loop-event-storage | team-analytics | [q4-loop-events-contract] | 2 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | 5000ed10db48 | [x] |
| gateway-loop-event-intake | team-gateway | [q4-loop-events-contract] | 2 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | f5d3bdb67a5e | [x] |
| frontend-loop-events | team-frontend | [gateway-loop-event-intake] | 2 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | 63d989578580 | [x] |
| frontend-ai-interaction-events | team-frontend | [gateway-loop-event-intake, frontend-loop-events] | 2 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | 033e124bd497 | [x] |
## Build order (cross_change_check)
1. q4-loop-events-contract (sequential, alone)  2. analytics-loop-event-storage + gateway-loop-event-intake (disjoint repos)  3. frontend-loop-events  4. frontend-ai-interaction-events
— hot-file pair that must not run in parallel: frontend-loop-events / frontend-ai-interaction-events (team-frontend/src/lib/analytics/schema.ts, map.ts, dataLayer.ts).
## Cross-change check
pass. Scenario titles (99) unique across all FEATURES.yaml, open changes, and the Q1 branch.
## Open issues / NEEDS_HUMAN
- Clarifications round 1 (2026-10-15, human): ai_interaction properties exactly {surface, action} else 400; magic_listing `dismissed` dropped; assistant `accepted` on both "Xem Ngay" and "Mua Ngay"; `agora_auth_evt` mirrors session-cookie options, not HttpOnly, Max-Age 60; `streak` only when > 0.
- DPO: classification of `agora_auth_evt` as functional/non-personal (no consent prompt); sign_up/login rows linked to principal id.
- Deploy order: team-analytics before (or with) gateway/frontend, else new types land as `unspecified` (no backfill).
- Accepted risk: forged sign_up/login beacons or a hand-set auth cookie can inflate counts (server-side delivery is a non-goal).
- Q1 conflict on team-frontend/src/features/auth/actions.ts, resolved after Q1 merges.
- Routing expected: proto (contract), pii (frontend slices → privacy-reviewer).

## Re-lock 2026-10-15 (build)
- frontend-ai-interaction-events: Clarifications Round 2 (human, chat) — this slice adds `ai_interaction` to GA4EventName/InternalEventType/map.ts; tasks.md 1.1 write-set widened. Digest c5c4f85a1fb4 → a76237442d7b.

## Re-lock 2026-10-16 (converge)
- All 5 changes: tasks.md boxes ticked.
- analytics-loop-event-storage, frontend-ai-interaction-events: Clarifications Round 3 (human) — VERIFIED BY / tasks e2e names aligned to spec scenario titles; text only.
- frontend e2e negatives keep a positive control step (human accepted).
- Digests: q4-loop-events-contract 3ec4a3ad52ef, gateway-loop-event-intake f5d3bdb67a5e, analytics-loop-event-storage 5000ed10db48, frontend-loop-events 63d989578580, frontend-ai-interaction-events 033e124bd497.
