# initiative-build q4-loop-instrumentation
integration: .claude/worktrees/q4-loop-instrumentation (asdlc/q4-loop-instrumentation, base asdlc/q3-audit-log @ 4821c30e)

| wave | slices | base | status |
|---|---|---|---|
| 0 | q4-loop-events-contract | fe9ef9b5 | merged 10b39e4c |
| 1 | analytics-loop-event-storage, gateway-loop-event-intake | 10b39e4c | merged ac7d11e9, ec84e04f |
| 2 | frontend-loop-events | ec84e04f | merged d34f0cc8 |
| 3 | frontend-ai-interaction-events | d34f0cc8 | merged 6691072b |

Per-slice: contract 1 round, analytics 1, gateway 1, frontend-loop 2 (beacon usePathname, biome), frontend-ai 2 (thread-switch reset; spec-clarify Round 2 re-lock: ai_interaction vocabulary).
Per-slice e2e at merge: analytics 4/4, gateway 21/21, frontend-loop 12/12, frontend-ai 16/16 (contract: not-testable, unit + buf).

## Converge (2026-10-16)
- features-check, spec_sync --strict ×5, openspec validate --strict ×5, cross_change_check, validate_plan: pass.
- CI: go vet/test team-gateway, team-analytics; platform-core buf lint/breaking + analytics/v1 tests; team-frontend vitest 1605/1605, tsc clean, biome 23 errors = base (none added), token lint 0.
- ci_local (BASE=asdlc/q3-audit-log): specs, features, repo, proto, tests-asdlc, security PASS; tdd PASS on contract range (q3..contract) and rest range (contract..HEAD) — no BSR failure this time.
- Regression subset (27 files: tracking/*, consent, ga4 datalayer/destination, tii_*, auth, register, favorites, follow, loyalty, engagement, chat, copilot, assistant, magic listing, notification alerts, reviews; stack q2): 129 passed, 3 failed, 1 error. All 3 failures reproduce on the Q3-base frontend image (pre-existing): emit_tracking duplicate view beacon (known since GA4 W1), tii event ids not distinct (same duplicate beacon), favorites "listing link hidden" locator. Error: reviews seed needs team-order (503, env-only).
- feature-reviewer (per slice, at merge): pass 15/15, 10/10, 19/19, 33/33, 22/22 (decide ENGINE_DOWN/LLM fallback). privacy-reviewer: needs-human ×2 (frontend slices).
