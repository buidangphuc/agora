# feature-run frontend-advertised-price
initiative: q5-advertised-price | base: f96772f9 | worktree: .claude/worktrees/frontend-advertised-price | branch: asdlc/frontend-advertised-price | started: 2026-10-16
## Preflight   validate=p spec_lint=p
## E2E         author=done run=red as expected 4/6 (2 green-before accepted as regression guards, Round 2 Q7); gateway gap found (CalculateShippingFee 501) → spec-clarify Round 2, task 0 added, re-locked
## Tasks
| task | scenarios | status | rounds | T0 | red_first | routed | task-reviewer | specialists |
|---|---|---|---|---|---|---|---|---|
| 0 + 1.1-1.5 | gateway fee route + PDP shipping + checkout server fee + price-changed UX | approved | 1 (+test cleanup: shipping.test.ts removed, orders.test.ts formatted, Red-First-Exempt) | 6f2c71eb | PASS | pii | approve | – |
## Gates       e2e=6/6 green (team-frontend:local + team-gateway:local) features-check=p spec_sync --strict=p (6 e2e + 17 unit) feature-reviewer=pass decide=ENGINE_DOWN (LLM fallback) privacy=needs-human (DPO: contract-necessity basis for PDP city display; deploy check: CDN must not cache authenticated /listing/*)
## Escalations spec-clarify Round 2 (human): gateway forwarder; regression guards accepted. Deviation accepted by reviewer: anonymous PDP renders a synchronous fallback (no calls). VERIFIED BY markers formatting fixed (re-lock note in LOCK.md).
merged: 6c4ec4a6
