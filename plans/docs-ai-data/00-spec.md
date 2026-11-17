# 00-spec — AI and data: honest forecast, experiments and reports, grounded assistant and copilot

initiative: docs-ai-data

Backlog: Q31 (ai#8, seller#8), Q16 (ai#4, growth#5, plus the reports and AI `request_id`/`model_version`
that Q4 deferred), Q15 (ai#1, ai#6; ai#5 and seller#5 split out, see U1), Q38 (doc drift, tasks only).
Source: PM review 2026-10-14 (`git show 1538115c:plans/pm-review-2026-10/`). Verified on origin/main 06c44c17.

## Goal
Sellers see a 14-day demand forecast for their best-selling listings on `/seller/analytics`, and
never see a made-up number: no forecast means "chưa đủ dữ liệu", an error means "tạm thời không có".
Every recommendation response names the generation, serving tier, ranker and experiment arm that
produced it, and admins read placement, retention, experiment and AI-surface reports in the cockpit.
The `/assistant` answer and the seller chat copilot are built from live catalog data (search hits,
listing stock and price) through team-ai, with the mock backend as the default, and every AI
response carries a `request_id` and `model_version` that the AI events record.

## Current state (verified)
- Q31: `DuckDBRepository.DemandForecast` returns `avgDaily = 2.0` on a query error and on zero history
  (team-analytics/internal/query/duckdb.go:163,169); `AVG(quantity)` is per order line, not per day
  (duckdb.go:150), and every day gets the same value.
  No frontend calls `GetDemandForecast` (seller app `src/lib/gateway/analytics.ts` has only funnel and
  revenue); `analytics.demand-forecast` still claims entry route `/seller/analytics`
  (team-analytics/FEATURES.yaml:113-129). platform-forecast is an unwired library with no FEATURES.yaml,
  image or job (platform-forecast/README.md "Known gaps"); it labels output `lgbm_quantile_v1` though no
  LightGBM model runs. ADR-0013 (Proposed) already decides: nightly job → Redis `fc:v1:seller:*` →
  served by team-analytics.
- Q16: no assignment anywhere in team-ai (no flipt/openfeature/experiment in team-ai/app). Wire
  `model_version` is the generation id only; the ladder floor `_popular_result` keeps the real model
  version (team-ai/app/modules/business/recommendation/service.py:172-174) and `fallback` reaches only the
  `recs.served` log line (servicers/recommendation.py:89-96). Flipt runs as shared infra and is used only
  for the boolean `checkout-enabled` kill switch (team-frontend/src/lib/flags/index.ts:19). No exposure
  event. No retention or cohort query (only the GA4 relay purge matches). `GetRecommendationPerformance`
  and `GetSearchPerformance` are admin-gated (team-gateway/internal/edge/policy.go:39-40) but no UI reads
  them; the cockpit shows only tracking quality (team-frontend-admin/src/features/admin/CockpitView.tsx:452).
  Done already: `EVENT_TYPE_AI_INTERACTION = 21` with `{surface, action}` (analytics.proto:284-286,
  collector.go:94-97, Q4); search `request_id`/`model_version` (search.proto:86-89, discovery-seo);
  the collector already forwards `impressionId`/`modelVersion` on any event (collector.go:266-267).
- Q15: `/assistant` overwrites the `StreamChat` text with the unary template reply
  (team-frontend/src/features/assistant/AssistantChatView.tsx:126). Retrieval is team-ai's own Qdrant RAG
  behind `RAG_ENABLED=false`, else `DemoCatalogRetriever` with invented ratings ("4.9/5 (1.2k đánh giá)",
  team-ai/app/modules/business/ai_assistant/catalog.py:13; factory.py:29). The reply is a fixed
  template (shopping.py `_build_assistant_reply`); the query log prints the raw `user_id` (shopping.py:32).
  The home `AiAssistantModal` invents `matchScore` 75-99 (team-frontend/src/features/ai/AiAssistantModal.tsx:123).
  The copilot always says "còn sẵn hàng" for stock questions and ignores `listing_id`
  (ai_assistant/copilot.py:28-44). Magic Listing ignores `image_url` (listing.py:17-38) and the seller app
  sends title and category only (team-frontend-seller/src/features/listing/MagicListingAssist.tsx:50).
  Reusable: `CHAT_BACKEND`/`SCREEN_BACKEND` `mock|llm_router` selectors (team-ai/app/core/config/ai.py:48-59),
  team-ai's team-search gRPC client (recommendation/adapters/search_stock.py), `SearchHit` with
  stock, sold_count, rating_avg, review_count (search.proto:64-79).

## User stories
1. **Forecast card.** Given seller S with ≥14 days of paid order lines on listing L and a forecast run
   after them, when S opens `/seller/analytics`, then a "Dự báo nhu cầu 14 ngày" card shows L's daily
   p10/p50/p90, the suggested reorder point and the model version and run date.
   Edge: L has no forecast key → the card says "Chưa đủ dữ liệu để dự báo" with no numbers.
   Edge: team-analytics cannot read the store → the card shows "Dự báo tạm thời không khả dụng"; the
   RPC returns `UNAVAILABLE`, never numbers.
2. **Nightly forecast job.** Given the order_facts Parquet export, when the forecast job runs, then it
   writes one `fc:v1:seller:<seller>:<listing>` key per listing with the model that actually ran, and a
   run summary (listings, cold starts, backtest WAPE vs the naive baseline). Edge: empty export → no
   keys written, status `empty`, exit 0.
3. **Recommendation arms.** Given an active Flipt variant flag for placement `home_feed`, when visitor V
   gets recommendations, then V always lands in the same arm, the arm's ranker is used and
   `model_version = <generation>/<tier>/<ranker>/<flag>:<variant>`.
   Edge: the ladder reaches the popular floor → tier is `popular`. Edge: Flipt unreachable → see U3.
4. **Reports.** Given an admin, when they open the cockpit, then they see: placements (recs and search
   impressions, CTR, add-to-cart, purchase per `model_version`), retention (weekly cohorts with D1/D7/D30
   and 30-day repeat-purchase rate), experiments (per variant: exposed, converted, rate, 95% Wilson CI),
   and AI surfaces (shown/accepted/edited/dismissed per surface and `model_version`).
   Edge: a non-admin calling any report RPC gets 403 at the edge and `PermissionDenied` in team-analytics.
5. **Grounded assistant.** Given published, in-stock listings matching "tai nghe dưới 500k", when a buyer
   asks `/assistant`, then the cards come from team-search (hybrid, `in_stock=true`, `max_price`) hydrated
   from team-domain, the reply cites only those cards' titles and prices, ratings come only from
   `rating_avg`/`review_count` (none shown when 0), and card impressions carry
   `placement_id=assistant.suggestions`, `impressionId=request_id`, `modelVersion`.
   Edge: team-search fails → the existing "chưa tra cứu được" reply with no cards. Edge: mock backend →
   the reply is the deterministic template over the real cards.
6. **Grounded copilot.** Given buyer message "còn hàng không?" on listing L with stock 0, when the seller
   opens suggestions, then no suggestion claims L is in stock. Edge: L unknown or the lookup fails →
   suggestions make no stock or delivery-time claim.

## Non-goals
- Q25 personalised home row and Q33 personalisation reach under consent: they wait for the consent
  decision (BACKLOG cross-PM conflict 4).
- Q15b photo-aware Magic Listing (multimodal input, attributes/variants, sold-price comparables) — see U1.
- Order-status questions in the assistant; streaming the grounded answer.
- Learning-to-rank, interleaving, multi-armed bandits, auto-promotion from readouts.
- Per-listing seller funnel table and the funnel range fix (seller#8 rest; `getSellerFunnel(me.id)` takes
  no range, team-frontend-seller/src/app/seller/analytics/page.tsx:31) — separate backlog item.
- Removing team-ai's own RAG (`RAG_ENABLED`, used by its `SearchListings` servicer) — one-line follow-up.
- Staging/prod gitops values beyond adding the forecast CronJob manifest (local first).

## Architecture decisions
- Forecast follows ADR-0013 (flip it to Accepted): platform-forecast gets `python -m forecast run`, a
  non-root image, a compose `forecast` profile job reading `analytics_data:/data/order_facts.parquet`
  (mirrors `featurestore-job`) and a platform-gitops CronJob; it writes Redis DB 3
  `fc:v1:seller:{seller}:{listing}` (TTL 48 h) plus `fc:v1:meta` (model_version, generated_at, WAPE).
  Why: decided in ADR-0013; one writer, no new port.
- Model choice per run: LightGBM quantile when backtest WAPE beats the seasonal-naive baseline on the
  same holdout, else the baseline; `model_version` names the one that ran. Listings below
  `MIN_HISTORY_DAYS_FOR_ML` get the baseline marked `is_cold_start`. Why: never label a model that did
  not run.
- team-analytics serves `GetDemandForecast` from the `fc:` keys only and deletes the DuckDB
  baseline: no key → `is_cold_start=true`, no points; Redis error → `UNAVAILABLE`. Reorder point math
  stays in `internal/rpc/handler.go`. Why: no fabricated numbers, no in-memory fallback (see U8).
- Assignment: team-ai evaluates Flipt variant flags over Flipt's REST evaluate API with the httpx it
  already ships (no new dependency); entity id = SHA-256 of the user or anonymous id plus a salt, so
  Flipt never sees raw ids. A placement names its flag in `placements.yaml`; a variant attachment may
  override only `ranking.model`. Why: one assignment system with the frontend; deterministic; see U2/U4.
- `model_version` on `RecommendResponse` becomes `<generation>/<tier>/<ranker>/<arm>`; tier ∈
  `user_precomputed|seed_vector_similarity|global_popular|popular|serving-fallback`, ranker ∈
  `gbdt|fixed_weights|similarity`, arm = `<flag>:<variant>` or `none`. No proto change: the string is
  opaque and analytics already groups by it.
- Frontend flag exposure: `getVariant(flag)` in team-frontend logs one `experiment_exposure` beacon
  `{flag, variant}` per page render (server-evaluated, allowlisted keys, no free text).
- Reports are three admin-only `AnalyticsQueryService` RPCs over DuckDB `tracking_events` +
  `order_facts`; output is aggregate counts only, never ids. Gateway forwards them with `adminScope`.
  Cockpit panels call them server-side from team-frontend-admin. CIs are computed in team-analytics
  (frontend holds no business logic).
- AI responses carry `request_id` (UUID hex, minted per call) and `model_version`
  (`<backend>:<prompt_version>` for LLM surfaces, `rules-v1` for rule surfaces, assistant appends
  `|<search model_version>`). The frontend passes them on `ai_interaction` beacons and card impressions.
- Assistant retrieval moves to team-search `SearchListings` (hybrid, `in_stock=true`, parsed max price,
  top 5) + `ListingService.GetListing` hydration in parallel; `DemoCatalogRetriever` and `catalog.py`
  are deleted. Copilot reads `ListingService.GetListing(listing_id)` and checks `seller_id` matches.
  LLM calls go through team-ai's existing LLM chain; new selectors `ASSISTANT_BACKEND` and
  `COPILOT_BACKEND` = `mock|llm_router`, default `mock` (see U10). Mock output is a deterministic
  template over the same retrieved facts, so e2e stays stable. Why: grounding is in the data path, not
  in the prompt only; mirrors `SCREEN_BACKEND`.
- `/assistant` uses the unary `ShoppingAssistant` only and stops calling `StreamChat` (see U9); the home
  `AiAssistantModal` matcher is deleted and becomes a link to `/assistant`.
- team-ai logs no raw user id (`shopping.py:32` fixed in the assistant slice).

## Contracts
```proto
// platform/ai/v1/ai.proto — additive fields
message ShoppingAssistantResponse { ... string request_id = 4; string model_version = 5; }
message MagicListingResponse      { ... string request_id = 7; string model_version = 8; }
message ChatCopilotResponse       { ... string request_id = 2; string model_version = 3; }
message SummarizeReviewsResponse  { ... string request_id = 5; string model_version = 6; }

// platform/analytics/v1/analytics.proto — EventType
// A server-evaluated feature-flag variant was shown. properties: exactly flag, variant.
EVENT_TYPE_EXPERIMENT_EXPOSURE = 22;

// GetDemandForecastResponse — additive
google.protobuf.Timestamp generated_at = 8;   // forecast run time; unset when is_cold_start

// add to AnalyticsQueryService (admin only)
rpc GetRetentionCohorts(GetRetentionCohortsRequest) returns (GetRetentionCohortsResponse);
rpc GetExperimentReadout(GetExperimentReadoutRequest) returns (GetExperimentReadoutResponse);
rpc GetAiSurfaceReport(GetAiSurfaceReportRequest) returns (GetAiSurfaceReportResponse);

message GetRetentionCohortsRequest {
  google.protobuf.Timestamp from = 1;  // first cohort week start (UTC Monday); unset = 12 weeks ago
  uint32 weeks = 2;                    // 1..26; 0 = 12
}
message RetentionCohort {
  string cohort_week = 1;              // YYYY-MM-DD (Monday)
  int64 cohort_size = 2;
  double d1 = 3; double d7 = 4; double d30 = 5;   // share active on day N (0..1)
  repeated double weekly_active = 6;   // index k = share active in week k after signup
  double repeat_purchase_30d = 7;      // share of cohort buyers with a 2nd paid order ≤30 d after the 1st
}
message GetRetentionCohortsResponse { repeated RetentionCohort cohorts = 1; }

message GetExperimentReadoutRequest {
  string flag = 1;                     // Flipt flag key, required
  google.protobuf.Timestamp from = 2;
  google.protobuf.Timestamp to = 3;
}
message ExperimentVariantRow {
  string variant = 1;
  int64 exposed = 2;                   // distinct visitors exposed
  int64 converted = 3;                 // of them, with a paid order within the window after first exposure
  double rate = 4;
  double ci_low = 5; double ci_high = 6;   // 95% Wilson interval
}
message GetExperimentReadoutResponse { repeated ExperimentVariantRow variants = 1; uint32 window_days = 2; }

message GetAiSurfaceReportRequest { google.protobuf.Timestamp from = 1; google.protobuf.Timestamp to = 2; }
message AiSurfaceRow {
  string surface = 1; string model_version = 2;
  int64 shown = 3; int64 accepted = 4; int64 edited = 5; int64 dismissed = 6;
}
message GetAiSurfaceReportResponse { repeated AiSurfaceRow rows = 1; }
```
Existing contracts consumed unchanged: `AnalyticsQueryService.GetDemandForecast`,
`AnalyticsQueryService.GetRecommendationPerformance`, `AnalyticsQueryService.GetSearchPerformance`,
`RecommendationService.Recommend`, `SearchService.SearchListings`, `ListingService.GetListing`,
`AIService.ShoppingAssistant|ChatCopilot|MagicListing|SummarizeReviews`, `analytics.events/TrackingEvent`.

## Capabilities → slices
| capability (user-observable) | slice id (= OpenSpec change) | owner | depends_on | uses / provides | repos touched |
|---|---|---|---|---|---|
| contracts | docs-ai-data-contract | platform-core | – | provides: 3 report RPCs, EXPERIMENT_EXPOSURE, AI provenance fields, forecast generated_at | platform-core |
| nightly forecast job with honest model label | forecast-nightly-job | platform-forecast | – | (Redis `fc:v1` writer) | platform-forecast, platform-gitops, docker-compose*, AGENTS.md |
| forecast served from job output, never fabricated | analytics-forecast-serving | team-analytics | contract, forecast-nightly-job | uses: GetDemandForecast | team-analytics, platform-e2e |
| seller forecast card (+ Magic Listing provenance on events) | seller-forecast-card | team-frontend-seller | analytics-forecast-serving, ai-copilot-grounding | uses: GetDemandForecast, AIService.MagicListing | team-frontend-seller, platform-e2e |
| recommendation arms + rich model_version | recs-experiment-arms | team-ai | – | uses: RecommendationService.Recommend | team-ai, platform-e2e |
| retention / experiment / AI-surface reports + gateway routes + exposure allowlist | analytics-experiment-reports | team-analytics | contract, analytics-forecast-serving (hot files) | uses: 3 report RPCs, analytics.events/TrackingEvent | team-analytics, team-gateway, platform-e2e |
| cockpit panels: placements, retention, experiments, AI surfaces | admin-insights-panels | team-frontend-admin | analytics-experiment-reports | uses: 3 report RPCs, GetRecommendationPerformance, GetSearchPerformance | team-frontend-admin, platform-e2e |
| grounded shopping assistant | ai-assistant-grounding | team-ai | contract, recs-experiment-arms (hot files) | uses: ShoppingAssistant, SearchListings, GetListing | team-ai, docker-compose.services.yaml, platform-e2e |
| grounded copilot + provenance on Magic Listing / review summary | ai-copilot-grounding | team-ai | contract, ai-assistant-grounding (hot files) | uses: ChatCopilot, GetListing, MagicListing, SummarizeReviews | team-ai, platform-e2e |
| buyer AI surfaces honest + measured, flag exposure beacon | buyer-ai-surfaces-ui | team-frontend | ai-assistant-grounding, ai-copilot-grounding, analytics-experiment-reports | uses: ShoppingAssistant, ChatCopilot, SummarizeReviews, analytics.events/TrackingEvent | team-frontend, platform-e2e |

Hot files edited by more than one slice (initiative-build serialises those slices):
- team-ai/app/core/config/ai.py, app/bootstrap/services.py — recs-experiment-arms, ai-assistant-grounding, ai-copilot-grounding
- team-ai/app/transport/grpc/servicers/ai.py — ai-assistant-grounding, ai-copilot-grounding
- team-analytics/internal/rpc/handler.go, internal/query/{repository.go,duckdb.go} — analytics-forecast-serving, analytics-experiment-reports
- docker-compose.services.yaml — forecast-nightly-job, analytics-forecast-serving (Redis DB 3 env), ai-assistant-grounding (listing addr)
- team-analytics/FEATURES.yaml — analytics-forecast-serving, analytics-experiment-reports

## Out of initiative
Q38 doc drift, verified item by item on 06c44c17 (rule: stale or historical docs are deleted, not fixed):
- [done] `plans/expand/INDEX.md`, `plans/features/INDEX.md` — already deleted.
- [done] `platform-core/docs/MASTER_CAPABILITY_MATRIX.md` — already deleted.
- [done] `orders.rma-modal` status — now `status: automated` (team-frontend/FEATURES.yaml:4874-4902).
- [done] `/account/referral` coins shown as đ — now `formatXu` (team-frontend/src/app/(shop)/account/referral/page.tsx:90-97).
- [done] FEATURES.yaml for team-referral and team-sharing — both exist.
- [task] Delete `plans/ai-first/PROGRAM.md`, `DECISIONS.md`, `C1-to-C2-handoff.md` (PROGRAM still says
  "draft … planning only", PROGRAM.md:3; ADR numbers wrong, :147-149). Historical; only reader is
  plans/discovery-followups/00-spec.md (a done plan, leave the citation).
- [task] Delete `openspec/ARCHIVE-STATUS.md` (generated 2026-10-11, lists `add-ga4-gtm-datalayer` as open)
  and drop its mention in docs/ONBOARDING.md:40 (`ls openspec/changes` is the status).
- [task] Delete `platform-core/docs/ROADMAP.md` (5 lines, one "shipped" note, no status icons left);
  readers are only done plans (q6, q11).
- [task] platform-core/docs/SECURITY_AND_RELIABILITY.md:15 says "HS256 / RS256" → "RS256" (live doc, one word).
- [task] Delete the dead `GenerateReferralCodeButton` (referral/page.tsx:77) and its
  `account.generating-a-code` entry (`status: not-testable`, team-frontend/FEATURES.yaml:2598):
  `GetMyReferral` always mints a code (team-referral/internal/referral/referral.go:57). UI removal, no
  requirement change.
- [slice, not task] platform-forecast FEATURES.yaml: lands with `forecast-nightly-job`, because the job
  gains observable behaviour that features-check must cover and the slice needs an owner FEATURES file.
- [slice] `analytics.demand-forecast` entry route `/seller/analytics` becomes true with `seller-forecast-card`.

## NFR and data (initiative level; each slice's proposal.md refines them)
- NFR: `GetDemandForecast` p95 ≤ 30 ms (one Redis GET); report RPCs p95 ≤ 2 s over 26 weeks on the local
  warehouse; Flipt evaluation adds ≤ 20 ms p95 to `Recommend`, timeout 50 ms; assistant p95 ≤ 1.5 s with
  the mock backend (search + ≤5 GetListing in parallel); copilot ≤ 300 ms mock. Forecast job ≤ 10 min for
  10k listings.
- Personal data: experiment entity id = salted SHA-256 of user/anonymous id (sent to Flipt, never logged
  raw); reports return aggregates only; AI events carry no free text (Q4 rule kept); the assistant and
  copilot prompts carry the redacted buyer message plus catalog facts, no buyer ids. Consent basis for
  retention/readouts: see U6. Retention: no new store; reads existing warehouse tables.
- Risk routing expected: `proto` (contract), `pii` (exposure entity hashing, reports), `auth` (new admin RPCs).

## Conventions
- Mirror `featurestore-job` (compose profile, non-root image, read-only `analytics_data`) for the forecast job;
  platform-forecast tests: `uv run pytest -q`; add a services-ci job like platform-featurestore.
- team-analytics report RPCs: mirror `GetChannelFunnel` (team-analytics/internal/rpc/handler.go:507: `requireAdmin`,
  repository capability interface, `Unavailable` when the warehouse lacks it); DuckDB↔BigQuery parity test.
- team-ai backends: mirror `build_screen_service` / `SCREEN_BACKEND`; gRPC clients like
  `adapters/search_stock.py` (service principal metadata); evals in team-ai/evals with `run_eval --min-score 0.8`
  get a groundedness set (reply prices/titles ⊆ cards).
- Frontend: assistant cards render through `ListingGrid` with a placement id; AI beacons via
  `trackAiInteraction` (team-frontend/src/lib/analytics/ai.ts); seller and admin apps keep their own
  gateway wrappers; Vietnamese copy.
- Never log raw user ids (sloglint forbidden keys; loguru in team-ai). Images run non-root.

## Decisions
- U1: Q15 is split. This initiative builds only the grounded assistant and the copilot; photo-aware Magic Listing (multimodal call, attributes/variants, sold-price comparables) is out of scope and moves to its own later initiative.
- U2: Experiment assignment uses Flipt variant flags evaluated by team-ai over REST; there is no salted-hash-in-placements.yaml fallback.
- U3: When Flipt is unreachable or slower than the 50 ms timeout during `Recommend`, the placement's default ranker is served and the response is stamped arm `unassigned`, which readouts exclude. The request never fails because of Flipt.
- U4: An arm may change the ranker only (`ranking.model`), not the candidate ladder or generation. This initiative creates the Flipt flags switched off and starts no live experiment.
- U5: The retention cohort anchor is the week of the user's first `sign_up` event; users who signed up before Q4 events existed are excluded.
- U6: Aggregate reports count all first-party events, as the existing funnels do, without filtering on `consent_analytics`. DPO sign-off on this basis is a human item.
- U7: The experiment conversion window is a paid order within 7 days of the visitor's first exposure.
- U8: A listing with no forecast key returns `is_cold_start` with no numbers; no DuckDB mean baseline is served.
- U9: `/assistant` shows the unary grounded `ShoppingAssistant` answer only and stops calling `StreamChat`; no grounded streaming RPC is added.
- U10: LLM backends use one selector per surface, `ASSISTANT_BACKEND` and `COPILOT_BACKEND`, each defaulting to `mock`.
- U11: The forecast card covers the top 5 listings by revenue in the selected range with a 14-day horizon; the forecast job runs nightly at 02:45, after the export.

## Locked
2026-11-13 — Confirmed by Bùi Đăng Phúc (all defaults accepted). Photo-aware Magic Listing deferred; Flipt variant-flag assignment (default ranker, arm `unassigned` on failure, ranker-only, flags off); first-sign_up retention anchor; all first-party events; 7-day conversion window; cold start without numbers; unary /assistant; per-surface mock-default backends; top-5 14-day forecast card, job at 02:45.
