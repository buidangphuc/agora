# LOCK — ga4-analytics
status: approved
approved-by: Bùi Đăng Phúc (chat, "lock.md ok")
date: 2026-10-12
spec: plans/ga4-analytics/00-spec.md (Locked 2026-10-11; Amendments 2026-10-12)
## Slices
| slice | owner | depends_on | rounds | validate | spec_lint | spec-quality | decide (spec) | digest | approve |
|---|---|---|---|---|---|---|---|---|---|
| ga4-consent-contract | platform-core | [] | 2 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | eb71262fc9f8 | [x] |
| add-consent-mode | team-frontend | [ga4-consent-contract] | 2 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | af12c65539e5 | [x] |
| add-ga4-destination | team-frontend | [add-consent-mode] | 2 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | 812a43e1b8be | [x] |
| add-ga4-export-import | team-analytics | [ga4-consent-contract] | 3 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | 8c2d52c39445 | [x] |
| add-ga4-purchase-relay | team-analytics | [ga4-consent-contract, add-ga4-destination] | 2 | pass | pass (1 PERM warn) | pass | ENGINE_DOWN (LLM fallback) | 5068777f68ed | [x] |
| recsys-consent-and-context | platform-recsys | [add-ga4-export-import] | 2 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | 028a133a90ec | [x] |
## Build order (cross_change_check)
1. ga4-consent-contract (contract, first)  2. add-consent-mode  3. add-ga4-destination  4. add-ga4-export-import  5. add-ga4-purchase-relay  6. recsys-consent-and-context
Hot-file pairs that must not run in parallel:
- gtm.ts, AnalyticsProvider.tsx: add-consent-mode → add-ga4-destination
- map.ts: add-ga4-destination → add-ga4-purchase-relay
- team-analytics/internal/export/export.go: add-ga4-export-import → recsys-consent-and-context
## Cross-change check
pass (cross_change_check: OK)
## Open issues / NEEDS_HUMAN
- decide.py: ENGINE_DOWN for every review (kev engine needs a --units/--state file that was not built) → spec-quality-reviewer checklist is the verdict.
- Routed pii: add-consent-mode, add-ga4-destination, add-ga4-purchase-relay (privacy-reviewer mandatory at the feature gate); proto: ga4-consent-contract; auth: GetChannelFunnel admin gate (add-ga4-export-import task gateway-funnel-gate).
- recsys-consent-and-context: `Recommend` is served by team-ai (team-ai/app/transport/grpc/servicers/recommend.py), outside the slice's repos. Denied buyers rely on the existing popular path; add team-ai to the slice if a code change is needed there.
- add-ga4-destination: CSP allows 4 Google hosts (adds *.analytics.google.com, region1.google-analytics.com) vs 2 in 00-spec. Also: identity route GET /api/analytics/identity, NEXT_PUBLIC_APP_ENV, GA4 retention 14 months, no purchase tag (gap until the relay lands).
- add-consent-mode: cookie Max-Age 180 d, not HttpOnly, SameSite=Lax, 256 B cap, flags independent, unknown version / missing or non-numeric ts → all denied. Open: should the frontend parser also treat duplicate keys as denied (gateway does)?
- add-ga4-purchase-relay: unconsented buyer → skip (amended); 5 attempts (1+4), backoff 1 s×2 cap 30 s; 4xx (not 429) → DLQ then commit; nil paid_at still sent without timestamp; mapping 30 d / ledger 90 d in relay-owned tables via the warehouse adapter; DLQ `analytics.ga4-relay.dlq`; order.events ACL in platform-gitops unverified. Open: accept the consent-revoke cross-topic race?
- add-ga4-export-import: reconcile threshold 0.35; `ga4_missing` adds a reason only (status unchanged); landing_page without query; ga4_sessions has export_date + ended_at; session window [ts, min(next ts, ended_at+30 min)).
- ga4-consent-contract: ga_client_id with analytics=false is blanked (event kept), no `GA1.1.` prefix; retention 13 months; service is AnalyticsQueryService.
- recsys: featurestore publishes als_interactions_v2 / rank_training_v2; denied events never touch per-user keys or item CTR counters; grant boundary = after the latest personalization=false event.
- 2026-10-12 re-lock ga4-consent-contract: VERIFIED BY added for 12 proto-level scenarios (spec-clarify), digest c8bf0ea24b29 → eb71262fc9f8, approved by Bùi Đăng Phúc (chat)
- 2026-10-12 re-lock add-consent-mode: tasks.md e2e-cls gains `capability: consent;` (spec_sync cross-check), digest 18796adbbe6d → 132381ced383, approved by Bùi Đăng Phúc (chat)
- 2026-10-12 re-lock add-ga4-export-import: plan gap — new requirement + task consumer-consent-cols (consumer persists ga_client_id + consent columns, 3 VERIFIED BY scenarios), digest 2481a1d0a73e → d074991e7614, approved by Bùi Đăng Phúc (chat)
- 2026-10-12 re-lock add-consent-mode (2): plan gap — edge copies beacon gaClientId into ga_client_id only under analytics consent, ≤64 chars (3 VERIFIED BY scenarios in gw-consent-stamp), digest 132381ced383 → 6f364dd36d11, approved by Bùi Đăng Phúc (chat)
- 2026-10-12 re-lock add-consent-mode (3): VERIFIED BY lines for 26 unit-verified scenarios (spec-clarify, text unchanged), digest 6f364dd36d11 → ad216e58bc03, approved by Bùi Đăng Phúc (chat, 'Thêm VERIFIED BY, re-lock')
- 2026-10-12 re-lock add-ga4-export-import (2): DuckDB single-writer → import runs inside the consumer (daily + internal trigger, CLI thin client, no CronJob), task ga4import-in-consumer, digest d074991e7614 → 3c13b271abe8, approved by Bùi Đăng Phúc (chat)
- 2026-10-12 re-lock add-ga4-export-import (3): VERIFIED BY lines for 15 unit-verified scenarios (text unchanged), digest 3c13b271abe8 → 8c2d52c39445, approved by Bùi Đăng Phúc (chat, 'Thêm VERIFIED BY, re-lock')
- 2026-10-12 re-lock add-consent-mode (4): privacy BLOCKING — no ecommerce push to dataLayer without analytics consent (incl. after revoke), new scenario, digest ad216e58bc03 → af12c65539e5, approved by Bùi Đăng Phúc (chat)
- 2026-10-12 re-lock recsys-consent-and-context: 028a133a90ec → 6d9869479e03 (spec-clarify R1: VERIFIED BY on 18 code-track scenarios, consent_denied via consumer log, dataset v2 path; approved by Bùi Đăng Phúc in chat)
- 2026-10-12 re-lock add-ga4-destination: 812a43e1b8be → 88f6794906cb (spec-clarify R1: identity route takes the principal only from new gateway edge GET /api/session/principal (verified JWKS + revocation); +2 scenarios, tasks 1.2b/2.2b; auth-scope BLOCKING on 1.2; approved by Bùi Đăng Phúc in chat)
- 2026-10-12 re-lock recsys-consent-and-context (2): 6d9869479e03 → 2872371067e0 (spec-clarify R2 Q5: empirical per-list propensity, smoothed, clamp [0.05,1], <100 impressions → position-only; +2 VERIFIED BY scenarios; approved by Bùi Đăng Phúc in chat)
- 2026-10-12 re-lock recsys-consent-and-context (3): 2872371067e0 → 7833ddb56212 (spec-clarify R3 Q6: context features evaluate-and-report only; published artifact keeps RANKING_FEATURES because team-ai accepts only that list; approved by Bùi Đăng Phúc in chat)
- 2026-10-12 re-lock add-ga4-purchase-relay: 5068777f68ed → 41d8a0b4a2fe (spec-clarify R1: VERIFIED BY for Mapping expires / Forged OrderPaidEvent / missing transaction id; old browser-purchase scenarios updated in this change; probe/container/metrics assumptions) — approved by Bùi Đăng Phúc in chat
- 2026-10-12 re-lock add-ga4-purchase-relay (2): 41d8a0b4a2fe → 2cc52bc0eff4 (spec-clarify R2 Q4: DLQ keeps the verbatim OrderPaidEvent incl. buyer_id for 7 days, documented in Data classification for DPO review) — approved by Bùi Đăng Phúc in chat
- 2026-10-12 re-lock add-ga4-purchase-relay (3): 2cc52bc0eff4 → 7343d50f78a1 (spec-clarify R2 Q5: consent withdrawal lag documented in Data classification for DPO review) — approved by Bùi Đăng Phúc in chat
