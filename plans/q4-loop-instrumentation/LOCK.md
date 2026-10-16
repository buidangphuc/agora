# LOCK — q4-loop-instrumentation
status: approved
approved-by: Bùi Đăng Phúc (chat "ok", 2026-10-15)
date: 2026-10-15
spec: plans/q4-loop-instrumentation/00-spec.md (Locked 2026-10-15)
## Slices
| slice | owner | depends_on | rounds | validate | spec_lint | spec-quality | decide (spec) | digest | approve |
|---|---|---|---|---|---|---|---|---|---|
| q4-loop-events-contract | platform-core | [] | 2 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | 0e01ce929270 | [x] |
| analytics-loop-event-storage | team-analytics | [q4-loop-events-contract] | 2 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | 9fbd3104f295 | [x] |
| gateway-loop-event-intake | team-gateway | [q4-loop-events-contract] | 2 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | 6f2058a25087 | [x] |
| frontend-loop-events | team-frontend | [gateway-loop-event-intake] | 2 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | f0e3660a66c6 | [x] |
| frontend-ai-interaction-events | team-frontend | [gateway-loop-event-intake, frontend-loop-events] | 2 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | c5c4f85a1fb4 | [x] |
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
