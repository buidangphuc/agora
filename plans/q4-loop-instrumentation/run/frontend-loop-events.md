# feature-run frontend-loop-events
initiative: q4-loop-instrumentation | base: ec84e04f | worktree: .claude/worktrees/frontend-loop-events | branch: asdlc/frontend-loop-events
## Preflight   validate=p spec_lint=p
## E2E         author=done run=red as expected (12/12: no event on analytics.events; 3 negative scenarios carry a positive control step) files=4 (+FEATURES 33)
## Tasks
| task | scenarios | status | rounds | T0 | red_first | routed | task-reviewer | specialists |
|---|---|---|---|---|---|---|---|---|
| 1.1-1.5 (batched, disjoint files) | 21 unit + 12 e2e | approved | 2 | bc170057 | PASS (FE buf plugins on PATH; round-2 lint-only test edits Red-First-Exempt) | auth, pii | approve (r2) | auth-scope CLEAN |
Round 1 → round 2: AuthEventBeacon history.pushState monkeypatch replaced by usePathname trigger (+ soft-navigation test); SameSite=Lax on expire; biome (CI linter, not prettier) errors fixed — slice adds none (23 = base).
## Gates       e2e=12/12 green (rebuilt image) features-check=p spec_sync --strict=p (12 e2e + 21 unit) feature-reviewer=pass 33/33 privacy=needs-human decide=ENGINE_DOWN (LLM fallback)
## Escalations privacy needs-human: DPO basis for identified first-party loop beacons without analytics consent; agora_auth_evt classification; Secure flag on session + auth_evt cookies (pre-existing); streak purpose.
