# initiative-plan — ga4-analytics
step: 5 — LOCK.md written (2026-10-12), waiting for human sign-off
cross_change_check: only `schema: no change directory` (expected before step 3); DAG ok
build order: ga4-consent-contract -> add-consent-mode -> add-ga4-destination -> add-ga4-export-import -> add-ga4-purchase-relay -> recsys-consent-and-context
deviation from 00-spec table: add-ga4-purchase-relay also depends_on add-ga4-destination (shared hot file map.ts)
| slice | author | round | validate | spec_lint | spec-quality | decide | cross-check |
|---|---|---|---|---|---|---|---|
| ga4-consent-contract | done | 1 | pass | pass | | | |
| add-ga4-purchase-relay | done | 1 | pass | pass (1 PERM warn: no inbound API) | | | |
| add-ga4-export-import | done | 1 | pass | pass | | | |
| add-ga4-destination | done | 1 | pass | pass | | | |
| add-consent-mode | done | 1 | pass | pass | | | |
| recsys-consent-and-context | done | 1 | pass | pass | | | |
## Open issues (round 1)
- ga4-consent-contract: capability name `tracking-consent` (author choice); compat-test location undecided (proto dir has no harness); rule "consumers drop ga_client_id when consent.analytics=false" is author-added (not in 00-spec) → human confirm; version-zero scenario is doc-only
- add-ga4-purchase-relay: idempotency/mapping store backend undecided (human); unconsented buyer → no purchase sent (conflicts with "exactly one per PAID") → human confirm; GA4 user_id not sent by MP → confirm; platform-gitops missing from repos touched (deployment, ExternalSecret, DLQ); assumed DLQ name/retries/TTLs; task covers: miss later negative scenarios (fix in round 2)
- add-ga4-export-import: MISSING INTERFACE channel funnel RPC (GetChannelFunnel) not in proto/contract slice → task blocked; human pick surface (add to ga4-consent-contract?); reconciliation via existing reasons strings + Prometheus gauge; threshold 0.35 default → confirm; landing_page without query string; ga4_sessions retention 13 months
- add-ga4-destination: user_id via new Next route GET /api/analytics/identity → human OK; no purchase tag (gap until relay lands); NEXT_PUBLIC_APP_ENV for staging, GA4 retention 14 months → confirm; CSP may need *.analytics.google.com, region1.google-analytics.com; consent helper shape assumed from 00-spec
- add-consent-mode: cookie lifetime 180d, not HttpOnly, SameSite=Lax → confirm; revoke can't unload GTM (consent update denied, off next load); unknown cookie version → v0 all denied; 256-byte cookie cap; flags independent (a=0,p=1 valid); ADDED-only tracking delta (no MODIFY of add-ga4-gtm-datalayer's GTM requirement); banner copy/i18n undecided
- recsys-consent-and-context: who owns export.go join of ga4_sessions (vs add-ga4-export-import); denied-actor popular path repo (platform-recsys recommend.py vs team-ai) unconfirmed; invented thresholds (100 impressions floor, view_cart 3.0, begin_checkout 6.0, grant boundary, ndcg&recall gate) → confirm; featurestore registry SQL may need dataset version bump; nearline item-level CTR from denied events (no user key) → privacy call
## Step 4 round 1
- 4a validate --strict: 6/6 pass; spec_lint: 6/6 0 errors (purchase-relay 1 PERM warn)
- 4b cross_change_check: ownership: recsys-consent-and-context (platform-recsys) touches capability `tracking` held by featurestore/analytics/frontend/gateway
- 4a spec-quality-reviewer x6 launched
- review add-ga4-destination: pass; decide=ENGINE_DOWN (LLM fallback; reviewer used system python without yaml); MINOR: HMAC-value wording, identity p95 scenario, withdrawal signal; QUESTION: 64-char ga_client_id cap vs contract
- review add-consent-mode: FAIL; BLOCKING cookie value `v=1;a=..` has raw `;` (breaks Cookie header) → needs encoding decision (00-spec Locked → human); Q: dangling "see open issue"; revoke = a=0 vs remove cookie
- review recsys-consent-and-context: FAIL; BLOCKING cold-start context can't reach Recommend (no RecommendRequest fields, "no new proto"); popular model-version id + serving repo undefined; nearline grant boundary/pre-grant state; MAY CTR counters → SHALL/SHALL NOT; no corrupt-message scenario; Q: grant boundary rule mismatch; 15-min begin_checkout idempotency window
- review add-ga4-purchase-relay: FAIL; BLOCKING mapping delete-on-denied contradiction; "retry 5" ambiguity + 429/backoff scenario; item field mapping (OrderLineItemFact has no name/category; value vs sum); PERM scenario (no inbound endpoint / who produces order.events); Q: retention basis for 30d/90d; DLQ topic owner
- review add-ga4-export-import: FAIL; BLOCKING channel-funnel RPC undeclared; gauge label event_type vs event; ga4_sessions lacks export_date; GetTrackingQualityReport admin gate unstated; Q: ga_client_id GA1.1. prefix
- review ga4-consent-contract: FAIL; BLOCKING "reserved numbers" scenario untestable now; version-zero THEN doc-only; Q: drop event vs blank ga_client_id when analytics=false
- decide.py: ENGINE_DOWN for all 6 (reviewers ran system python without pyyaml) → LLM fallback
- round 1 result: 1 pass (add-ga4-destination), 5 fail → step 4d human questions before round 2
## Step 4d (2026-10-12) human answers → 00-spec "Amendments 2026-10-12"
- cookie `&` separators; GetChannelFunnel added to ga4-consent-contract (slices.yaml provides/uses updated); no request-time recsys boost; relay skips unconsented buyers
## Round 2 — 6 team-spec-author revisions launched (destination only for alignment)
- r2 add-ga4-destination: done; validate/lint pass; LOCK note: CSP +2 hosts deviation
- r2 add-ga4-export-import: done; validate/lint pass; all 4 BLOCKING addressed
- r2 ga4-consent-contract/add-consent-mode/add-ga4-purchase-relay/recsys-consent-and-context: done, validate/lint pass (relay 1 PERM warn)
- service name corrected to AnalyticsQueryService (00-spec, slices.yaml); export-import author asked to rename + align ChannelFunnelRow fields
- r2 spec-quality-reviewer x5 launched (export-import after rename)
- LOCK notes: nil paid_at still sent w/o timestamp; gitops write-set (ga4-relay.yaml, redpanda.yaml ACL unverified); 4xx → DLQ + commit; missing/non-numeric ts → cookie malformed; Recommend serving repo unconfirmed; decode_errors counter name
## Round 2 reviews (decide=ENGINE_DOWN for all: kev engine exit 2/3 even with venv python → LLM fallback)
- ga4-consent-contract PASS (minor: consent comment scenario, from/to unset rules, buf breaking category)
- add-consent-mode PASS (minor: 180d expiry scenario, FE duplicate-key parity; Q: FE treat duplicate keys as denied?)
- add-ga4-destination PASS (minor: img-src assertion; Q: CSP 4 hosts vs 00-spec 2)
- add-ga4-purchase-relay PASS (1 PERM warn; minor: counter names, 90d ledger scenario, DLQ redelivery wording; Q: consent-revoke cross-topic race)
- recsys-consent-and-context PASS (minor: ga4_sessions join key/which session wins). FACT: Recommend is served by team-ai (team-ai/app/transport/grpc/servicers/recommend.py) → denied-buyer popular path / coldstart write-set is outside slice repos → NEEDS_HUMAN at LOCK
- add-ga4-export-import r2 review FAIL: no gateway edge-gate task for GetChannelFunnel; session end needs ended_at → round 3 sent (default: ga4_missing = reason only, status unchanged)
- add-ga4-export-import r3 review PASS; cross_change_check OK; LOCK.md pending
- 2026-10-12: #9 merged by human; origin/main merged into feat/ga4-analytics (bd3f14bf), pushed; waiting #8 CI → merge (human) → openspec archive add-ga4-gtm-datalayer
- 2026-10-12: #8 merged by human; add-ga4-gtm-datalayer archived on chore/archive-add-ga4-gtm-datalayer → PR #10 (fe-datalayer ticked, 3 not-testable FEATURES entries). Next: LOCK.md sign-off → initiative-build
