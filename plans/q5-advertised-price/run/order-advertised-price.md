# feature-run order-advertised-price
initiative: q5-advertised-price | base: b4642001 | worktree: .claude/worktrees/order-advertised-price | branch: asdlc/order-advertised-price | started: 2026-10-16
## Preflight   validate=p spec_lint=p
## E2E         author=done run=red as expected (5/6; buyer-token scenario green before = regression guard)
## Tasks
| task | scenarios | status | rounds | T0 | red_first | routed | task-reviewer | specialists |
|---|---|---|---|---|---|---|---|---|
| 1-4 | proto/upstream/cart price/reprice | approved | 1 (+fake no-op, Red-First-Exempt) | 2ad46816 | PASS | auth | approve | auth-scope PASS |
| 5-10 | pre-check, Phase A/B, compensation, cancel, handler, docs | approved | 2 (nil listing fail-closed with flash wired; cancel flash release own timeout) | a56f20c4 / 0f717db2 | PASS | payment-adjacent | approve | – |
## Gates       e2e=6/6 green (+ promotion 14/14 regression) features-check=p spec_sync --strict=p (6 e2e + 43 unit) feature-reviewer=pass decide=ENGINE_DOWN (LLM fallback)
## Escalations none. Follow-ups: add the 3 hold RPCs to upstream methodScopes safety net (domain.go, outside write-set); identity/promotion README docs list promotion.reserve as voucher-only; nil listing fails closed only when flash is wired (legacy fake returns nil listing).
merged: f96772f9
