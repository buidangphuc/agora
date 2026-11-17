# LOCK — docs-ai-data
status: approved
approved-by: Bùi Đăng Phúc
date: 2026-11-13
spec: plans/docs-ai-data/00-spec.md (Locked 2026-11-13)

## Slices
| slice | owner | depends_on | rounds | validate | spec_lint | spec-quality | decide (spec) | digest | approve |
|---|---|---|---|---|---|---|---|---|---|
| docs-ai-data-contract | platform-core | [] | 1 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | 3a9d21916b42 → 8365f87a8ba0 | [x] |
| forecast-nightly-job | platform-forecast | [] | 1 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | 19b344b05766 → 1ef28e2ca471 | [x] |
| analytics-forecast-serving | team-analytics | [docs-ai-data-contract, forecast-nightly-job] | 1 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | d53c8e85412e → 2b5ec0e327db | [x] |
| recs-experiment-arms | team-ai | [] | 1 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | 31f1eb885578 → d0d3b3fb68d1 | [x] |
| analytics-experiment-reports | team-analytics | [docs-ai-data-contract, analytics-forecast-serving] | 1 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | c06fea1873ba → 7994db0f075c | [x] |
| ai-assistant-grounding | team-ai | [docs-ai-data-contract, recs-experiment-arms] | 1 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | 6e5b9b48e616 → 343d735f6f25 | [x] |
| ai-copilot-grounding | team-ai | [docs-ai-data-contract, ai-assistant-grounding] | 1 | pass | pass (1 PERM warning, false positive) | pass | ENGINE_DOWN (LLM fallback) | f1c635233431 → dfc3e8444b6b | [x] |
| seller-forecast-card | team-frontend-seller | [analytics-forecast-serving, ai-copilot-grounding] | 1 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | c22f5e119ead → 1e60e2e9b35a | [x] |
| admin-insights-panels | team-frontend-admin | [analytics-experiment-reports] | 1 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | 4f298a3bd310 → 165b23297301 | [x] |
| buyer-ai-surfaces-ui | team-frontend | [ai-assistant-grounding, ai-copilot-grounding, analytics-experiment-reports] | 1 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | 89bc443965e2 → 8b93728ef3f4 | [x] |

Digest update 2026-11-16 (converge): every slice's openspec/changes/<slice>/** changed (tasks.md ticks; VERIFIED BY citation lines in spec.md for forecast-nightly-job, ai-assistant-grounding, ai-copilot-grounding, admin-insights-panels and buyer-ai-surfaces-ui; renamed e2e scenario names in tasks.md); the digest cell shows old → new.

Digest update 2026-11-16 (CI): two scenario titles renamed so each matches exactly one FEATURES.yaml (cross_change_check ownership), wording of the steps unchanged: ai-copilot-grounding `Unknown listing` → `Copilot stock question for an unknown listing` (collided with team-promotion), admin-insights-panels `Anonymous visitor is redirected` → `Anonymous visitor is redirected from the cockpit` (collided with team-frontend).

Re-approval 2026-11-16: approved-by Bùi Đăng Phúc 2026-11-16 "batch re-approval after converge (decision 5)". Digests verified on asdlc/docs-ai-data at ba2f52da8: all 10 slices match the new digest in the table. Reasons: ai-copilot-grounding dfc3e8444b6b (scenario title renamed to match one FEATURES manifest); admin-insights-panels 165b23297301 (scenario title renamed to match one FEATURES manifest); every other slice: converge tasks.md ticks, VERIFIED BY citation lines and renamed e2e scenario names in tasks.md, no requirement wording change.

## Known exceptions
- validate_plan wave 4/5/6 widened conflicts (decision 9, 2026-11-16): validate_plan.py read the `- widened (P1)` bullet of each packet as a write-set entry named `widened`, so every same-wave packet pair conflicted: wave 4 analytics-experiment-reports / ai-assistant-grounding, wave 5 ai-copilot-grounding / admin-insights-panels, wave 6 seller-forecast-card / buyer-ai-surfaces-ui. The widened paths themselves do not overlap within a wave. The packet header is plain text since 99ff6c459, and the parser fix (widened entries parsed, marker-only pyproject edits tolerated) is in the tooling PR. `validate_plan.py plans/docs-ai-data` reports the plan valid (10 packets, 7 waves) with both the branch copy and the tooling copy.

decide.py was ENGINE_DOWN for every slice (no Kev engine or units file; system python lacks `yaml`). Each verdict is the spec-quality-reviewer's own checklist.

## Build order (cross_change_check)
1. docs-ai-data-contract (contract, sequential)
2. forecast-nightly-job
3. analytics-forecast-serving
4. analytics-experiment-reports
5. admin-insights-panels
6. recs-experiment-arms
7. ai-assistant-grounding
8. ai-copilot-grounding
9. buyer-ai-surfaces-ui
10. seller-forecast-card

Hot-file pairs that must not run in parallel (00-spec):
- team-ai/app/core/config/ai.py, app/bootstrap/services.py: recs-experiment-arms, ai-assistant-grounding, ai-copilot-grounding
- team-ai/app/transport/grpc/servicers/ai.py: ai-assistant-grounding, ai-copilot-grounding
- team-analytics/internal/rpc/handler.go, internal/query/{repository.go,duckdb.go}: analytics-forecast-serving, analytics-experiment-reports
- docker-compose.services.yaml: forecast-nightly-job, analytics-forecast-serving (Redis DB 3 env), ai-assistant-grounding (listing addr); recs-experiment-arms also needs two env lines here (open issue below)
- team-analytics/FEATURES.yaml: analytics-forecast-serving, analytics-experiment-reports

## Cross-change check
OK (exit 0), 0 violations (initial run had 2; fixed by adding the initiative line and renaming a duplicate scenario in ai-copilot-grounding).

## Open issues / NEEDS_HUMAN

### Escalated slices
- None at round 3. recs-experiment-arms and ai-copilot-grounding need one more round for the cross-check violations.

### Spec-changing questions
1. Redis DB 3: docker-compose.services.yaml:718 says DB 3 is used by "ofb tests". Confirm DB 3 is free for `fc:v1`, or pick another DB (forecast-nightly-job and analytics-forecast-serving must agree).
2. New dependency go-redis/v9 in team-analytics (no go-sdk Redis package). Approve.
3. New dependencies lightgbm, duckdb, libgomp1 in platform-forecast. Approve; env names stay unprefixed (REDIS_URL, REDIS_PREFIX, REDIS_TTL_SECONDS).
4. ADR-0013 status flip in platform-core/docs/ADR (forecast-nightly-job task 1.8) is outside that slice's repos. Allow it.
5. Forecast `empty` run writes no `fc:v1:meta`, so a long empty export shows as "no key" only after the 48 h TTL. Accept.
6. Cold start has two shapes: a key with baseline points and `is_cold_start=true`, and no key with no points (U8). Confirm, and say whether `suggested_reorder_point` and `safety_stock` come from the baseline points or are 0 for a stored cold-start key.
7. Forecast window starts at date(AS_OF) on complete UTC days, not the day after the latest input date. Confirm.
8. Is the forecast job's 10 min / 10k listings NFR gate-checked, or only a stated budget?
9. analytics-forecast-serving assumptions: `horizon_days` defaults to 14 and is clamped to 1..28; a malformed or over-64 KiB payload returns UNAVAILABLE; ids containing `:` or `*` return InvalidArgument; the payload layout stays as in platform-forecast/README.md and publisher.py. Confirm.
10. seller-forecast-card: does the card send `lead_time_days` and `service_level`, or use server defaults? Proto zero values could give a different reorder point than the e2e expects.
11. seller-forecast-card: top 5 listings come from a range-scoped `GetRevenueBreakdown(from,to)`. Confirm team-analytics honours from/to for topSkus.
12. Report RPCs: weeks > 26 returns INVALID_ARGUMENT, and the conversion window is fixed at 7 days (`window_days` response field). Confirm both are part of the contract.
13. Contract requires the literal text `scope "admin"` in rpc comments (brittle match). Accept.
14. Retention metrics for a cohort too young to measure are reported as 0 (no presence flag in the proto), so the admin UI cannot tell "not yet measurable" from zero. Accept, or add a field in a later contract change.
15. Exposure beacon value pattern `^[a-z0-9][a-z0-9_.-]{0,63}$`. Confirm it covers the real Flipt flag and variant keys.
16. Recs tier values: the locked list omits `category_popular` and `cart_cross_similarity` (real ladder strategies). Confirm the wider list.
17. Recs arm on Flipt failure: `<flag>:unassigned` instead of bare `unassigned`. Confirm.
18. Recs `model_version` when the serving generation is unreadable: `serving-fallback/serving-fallback/similarity/<arm>`. This changes an existing e2e that expects exactly `serving-fallback`. Confirm.
19. `RECS_FLIPT_URL` and `RECS_EXPERIMENT_SALT` go into docker-compose.services.yaml (hot file not in recs-experiment-arms' repos). Allow this slice to add them, or name the slice that does.
20. Which environments get the `recs-home-feed-ranker` flag beyond the local seed script (ops item).
21. Assistant: a card is kept only if GetListing returns PUBLISHED; stock is trusted from the search `in_stock` filter; search read-model lag can briefly show an out-of-stock listing. Accept.
22. team-ai service principal with `listing.read` (GetListing) and `search:read` (SearchListings, via RECS_SEARCH_ADDR) must be accepted by both services in every environment.
23. Assistant `prompt_version` = `assistant-v1`, so `model_version` = `mock:assistant-v1|<search model_version>`. Confirm.
24. Copilot `model_version`: `rules-v1` on mock and `llm_router:<prompt_version>` on LLM (not `mock:<prompt_version>`). Confirm.
25. Copilot timeouts: COPILOT_TIMEOUT_SECONDS default 3 s and GetListing timeout 200 ms (not in 00-spec). Confirm.
26. Copilot invented claims beyond stock and delivery (100% chính hãng, warranty, 7-day returns, free-shipping vouchers) stay ungrounded. Follow-up to remove or ground them?
27. Copilot admin impersonation: when an admin calls ChatCopilot with another seller's seller_id, ground on that seller's listing? If yes, add a scenario.
28. Copilot delivery-address ask: fixed phrase or defined matcher for the assertion?
29. `dưới 500k` includes exactly 500000 (inclusive). Intended? And state the rounding mode for rating_avg (4.25 -> 4.2 needs half-even).
30. `placement_id=assistant.suggestions` on card impressions is set by buyer-ai-surfaces-ui. Confirm.
31. Duplicate retirement: both ai-assistant-grounding (assumed) and buyer-ai-surfaces-ui remove the "reply streams token-by-token" requirement; keep it in buyer-ai-surfaces-ui only.
32. Anonymous visitors get no flag variant (anonymous id only in localStorage), narrowing 00-spec's "user or anonymous id". Accept signed-in only, or add a server-readable anonymous-id cookie (consent and Secure-cookie question).
33. No page calls getVariant in this initiative (flags off, U4), so experiment_exposure is unit-tested only. Want a real consumer page or flag for an e2e?
34. Frontend getVariant budget (50 ms timeout, 20 ms p95) is copied from team-ai. Confirm.
35. `request_id` and `model_version` travel only in impressionId/modelVersion wire fields, never in properties (relaxes the old ai-interaction-events rule). Confirm. The `dismissed` action is never emitted by buyer surfaces, so that column stays 0 for them.
36. Magic Listing beacon provenance: itemId is the listing id or empty on the create form. Confirm team-analytics accepts an event with empty listing_id and a set impression_id.
37. Admin experiment panel needs a typed Flipt flag key (no flag-list RPC). Accept, or name the flag to pre-fill.
38. Admin AI-surface accepted/dismissed share is a UI-computed ratio. Accept, or add it to the contract.
39. Admin panels live on /admin/cockpit (server-rendered, not polled, URL query selectors) rather than a new /admin/insights route. Confirm.
40. A PERMISSION_DENIED on any of the five admin RPCs turns the whole page into 403, not one panel. Confirm the UX.
41. Shared VERIFIED BY `TestReports_NonAdminDenied` covers three RPCs: confirm it is a table test with one assertion per RPC; anonymous 401 is unit-tested only (`TestPolicy_ReportProceduresRequireAuthentication`). Enough?
42. team-gateway and team-frontend-admin need refreshed vendored proto copies in the contract slice even though their routes ship later. Confirm.

### Legal / DPO and accepted risks (not spec-changing)
- U6 consent basis: aggregate reports count all first-party events without `consent_analytics` filtering. DPO sign-off open (contract, reports, recs entity hash, admin panels, seller beacons).
- Readouts have no small-sample suppression threshold; admin-only access and aggregate shape are the only mitigation. DPO to confirm with U6.
- Retention anchors on authenticated user ids only (U5); anonymous sign_up events fall outside every cohort.
- Beacon retention and consent basis for impressionId and modelVersion defer to U6.

### Reviewer MINOR items to fold in during build (no human decision)
- ai-copilot-grounding: add scenarios for the 1000/3/300-character limits, the variant-sum stock rule, and the LLM delivery-claim fallback; move "Default drafts carry no availability claim" under stock grounding.
- recs-experiment-arms: add scenarios for the 20 ms Flipt NFR, 404 -> `none` and unparsable body -> `unassigned`; avoid a wall-clock 150 ms assertion.
- analytics-experiment-reports: add "with `from` set to the Monday of that cohort week" to the young-cohort scenario.
- admin-insights-panels: add a page.test.tsx redirect case; align the model_version wording in design.md:11 with 00-spec; use a fixed clock for the range scenario.
- buyer-ai-surfaces-ui: design.md:9 points at a missing "open issue 1"; add an Open issues entry.
- Build gate: analytics-experiment-reports, ai-copilot-grounding and buyer-ai-surfaces-ui need docs-ai-data-contract merged first (proto fields not yet in platform-core).
