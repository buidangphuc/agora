# LOCK — ga4-analytics
status: pending            # human sets: approved | rejected
approved-by:
date:
spec: plans/ga4-analytics/00-spec.md (Locked 2026-10-11; Amendments 2026-10-12)
## Slices
| slice | owner | depends_on | rounds | validate | spec_lint | spec-quality | decide (spec) | digest | approve |
|---|---|---|---|---|---|---|---|---|---|
| ga4-consent-contract | platform-core | [] | 2 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | c8bf0ea24b29 | [ ] |
| add-consent-mode | team-frontend | [ga4-consent-contract] | 2 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | 18796adbbe6d | [ ] |
| add-ga4-destination | team-frontend | [add-consent-mode] | 2 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | 812a43e1b8be | [ ] |
| add-ga4-export-import | team-analytics | [ga4-consent-contract] | 3 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | 2481a1d0a73e | [ ] |
| add-ga4-purchase-relay | team-analytics | [ga4-consent-contract, add-ga4-destination] | 2 | pass | pass (1 PERM warn) | pass | ENGINE_DOWN (LLM fallback) | 5068777f68ed | [ ] |
| recsys-consent-and-context | platform-recsys | [add-ga4-export-import] | 2 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | 028a133a90ec | [ ] |
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
