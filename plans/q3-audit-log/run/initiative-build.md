# initiative-build q3-audit-log
integration: .claude/worktrees/q3-audit-log (asdlc/q3-audit-log, base asdlc/q2-review-integrity @ 1cd37ad7; spec commit cf500af5)

| wave | slices | base | status |
|---|---|---|---|
| 0 | q3-audit-log-contract | cf500af5 | merged 52442928 |
| 1 | audit-query-filters | 52442928 | merged 17bb9153 |
| 2 | verification, engagement, domain, order, payment, promotion, frontend (WIP 3) | 17bb9153 | merged e3130cec, e441a9b6, 75ac9416, 1c815647, 1a980582, 381e2a43, d9107288 |

Per-slice e2e at merge: filters 14/14, verification 4/4, engagement 4/4, domain 4/4, order 7/7, payment 4/4, promotion 9/9, frontend 9/9.
Red-first: BSR rate limit → manual base/head runs with local buf plugins (recorded in merge messages); gateway forwarder and FE cockpit 403 guard use Red-First-Exempt trailers.

## Converge (2026-10-15)
- spec_sync --strict, openspec validate --strict, cross_change_check, features-check: pass (FEATURES fixes 65d3ea5b).
- feature-reviewer: pass 9/9 (decide=ENGINE_DOWN, LLM fallback). privacy-reviewer: needs-human (dispute note in immutable audit rows; no team-audit retention/erasure).
- Regression subset (order/payment/promo/listing/dispute/kyc/edge/review/audit, 327 tests on compose project q2): 42 failed first run; triage:
  - env-only (agora-* container/image defaults, services not started: search/chat/notification/analytics/ai/flipt/minio, SAVE10 seed absent, payment-ledger and order-inventory overlays inactive, uif assert_agora guard): 41.
  - rerun of payment/saga/payout files with PLP_*/DOMAIN_* overrides: 80 passed, 5 failed → 4 env (overlays inactive ×3, boot_guard_payment pre-existing DATABASE_URL-before-MOCK check, unrelated to Q3).
  - real regression: prm_pay legacy refund scenario got `audit unavailable` (throwaway team-payment lacked UPSTREAM_AUDIT_ADDR) → fixed in payment slice 64fcbcbc (prm_pay_stack PRM_PAY_AUDIT_ADDR), 20/20 green, re-merged.
- Index check (audit-query-filters task 6): 100k rows; action-only → idx_audit_events_action_created (+ incremental sort on id, 0.04 ms); target → idx_audit_events_target_created; actor+time → idx_audit_events_actor_created.
