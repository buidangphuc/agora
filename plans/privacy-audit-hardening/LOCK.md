# LOCK — privacy-audit-hardening
status: approved
approved-by: Bùi Đăng Phúc
date: 2026-10-18
spec: plans/privacy-audit-hardening/00-spec.md (Locked 2026-10-18)

## Slices
| slice | owner | depends_on | rounds | validate | spec_lint | spec-quality | decide (spec) | digest | approve |
|---|---|---|---|---|---|---|---|---|---|
| pah-contract | platform-core | [] | 2 | pass | pass | pass (r2 fixed team-order events.proto copy) | ENGINE_DOWN (LLM fallback) | 0500d3f4f471 | [x] |
| identity-session-retention | team-identity | [] | 1 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | d422d50cf33a | [x] |
| frontend-cookie-secure | team-frontend | [] | 1 | pass | pass (2 warnings) | pass | ENGINE_DOWN (LLM fallback) | 5ecea04c5ff4 | [x] |
| frontend-session-client-context | team-frontend | [frontend-cookie-secure] | 1 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | e4674c3f53c8 | [x] |
| frontend-referrer-hygiene | team-frontend | [frontend-cookie-secure] | 1 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | faeb36f278a0 | [x] |
| domain-consumer-key-redaction | team-domain | [] | 1 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | bd8ae61894fe | [x] |
| engagement-consumer-key-redaction | team-engagement | [] | 1 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | c5db4cc1397b | [x] |
| payment-refund-audit-id | team-payment | [] | 1 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | 6a2b1dce266d | [x] |
| promotion-shop-voucher-audit | team-promotion | [pah-contract] | 1 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | fd53448938e1 | [x] |
| analytics-review-hidden | team-analytics | [pah-contract] | 2 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | 1fef70044c54 | [x] |
| engagement-review-hidden-event | team-engagement | [pah-contract, analytics-review-hidden, engagement-consumer-key-redaction] | 1 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | 6b8a5849f625 | [x] |
| featurestore-hidden-review-exclusion | platform-featurestore | [analytics-review-hidden] | 1 | pass | pass (1 n/a warning) | pass | ENGINE_DOWN (LLM fallback) | 4913598d53a0 | [x] |

## Build order (cross_change_check)
1. pah-contract (sequential, alone)
2. analytics-review-hidden, domain-consumer-key-redaction, engagement-consumer-key-redaction, frontend-cookie-secure, identity-session-retention, payment-refund-audit-id, promotion-shop-voucher-audit
3. engagement-review-hidden-event, featurestore-hidden-review-exclusion, frontend-session-client-context, frontend-referrer-hygiene
Hot-file pairs that must not run in parallel: frontend-session-client-context / frontend-referrer-hygiene / frontend-cookie-secure (src/middleware.ts, src/features/auth/actions.ts, analytics gtm.ts); the two team-engagement slices (same FEATURES.yaml).

## Cross-change check
pass

## Open issues / NEEDS_HUMAN
Decisions made during spec (orchestrator/authors, confirm or change):
- identity: new migration `0008_session_revoked_at` (sessions has no revoke time); legacy revoked rows age from `last_seen`; un-revoked rows also purge 30 days after the existing 90-day absolute session lifetime. Env names SESSION_RETENTION=720h, LOGIN_HISTORY_RETENTION=4320h, SESSION_SWEEP_INTERVAL=1h, SESSION_SWEEP_BATCH_SIZE=500, SESSION_SWEEP_ENABLED=true; invalid values fail startup.
- analytics: nullable `review_id` stored on both review_created and review_hidden facts; review_created rows written before deploy keep NULL, so hides of pre-deploy reviews are recorded but not excluded downstream (no backfill). review_hidden time = hidden_at, else envelope time.
- featurestore: new `item_popularity@v2` and `als_interactions@v3` (locked versions untouched); recsys still reads v1 by default and item_popularity@v1 readers keep counting hidden reviews until re-pinned (not in this initiative).
- promotion: voucher.admin_create metadata = discount_type, value, seller_id.
- payment: a retried admin refund writes a second audit row with the same refund_id (no audit dedup).
- frontend: COOKIE_SECURE forces only on case-insensitive "true"; agora_auth_evt now follows the same Secure rule (small behaviour change); /admin match is case-insensitive and segment-exact, fragment stripped with the query; revoke forwards client IP/UA for gateway attribution only (identity stores nothing new on revoke).
- pah-contract: audit.proto catalogue comment refreshed in all 11 vendored copies; events.proto in team-engagement, team-analytics, team-order.
- engagement: HideReview p99 <= 10 ms NFR has no benchmark task (non-binding unless you want one).
Routing: auth (cookies, sessions, admin routes), pii (sessions, logs, analytics facts), proto (contract) → NEEDS_HUMAN at build step 5.
DPO (human, not code): lawful basis/retention for key_fp, refund_id in immutable audit rows, client IP/UA updated on refresh, review_hidden facts and already-built datasets.
Deploy order: pah-contract consumers regenerate; team-analytics before team-engagement (unknown event types are dead-lettered); platform-gitops COOKIE_SECURE with the frontend release.
