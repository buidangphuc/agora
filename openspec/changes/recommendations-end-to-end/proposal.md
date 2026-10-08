## Why

This is change **C3** of the AI-first marketplace program (`plans/ai-first/PROGRAM.md`, binding names). The
site exists to serve recommendations, but the recommendation pipeline has never run end to end, nothing
checks a model before users see it, and nothing measures whether what users saw was any good. Verified in
code (2026-10-01):

- **No governed input.** The trainer reads raw `tracking_events` (DuckDB Parquet or BigQuery) through
  `WAREHOUSE_PARQUET_PATH`, an `emptyDir` nothing fills; `team-analytics` `ExportParquet` is uncalled. Under
  the program, behavioural data and features belong to `team-analytics` (C2 `analytics-feature-store`) and
  a consumer reads a declared feature view, never raw events (PROGRAM principle 3).
- **No in-cluster run.** The `platform-recsys` CronJob points at `qdrant:6333` / `redis:6379`;
  `platform-gitops` deploys no Qdrant and its Redis is `valkey`. `team-ai`'s deployed env sets no `RECS_*`
  and `REDIS_ENABLED=false`.
- **No safety on publish.** The trainer overwrites Redis keys in place, prunes Qdrant before flipping
  `recs:v1:model_version`, has no offline metric, no gate (an empty or degenerate model is published) and
  no rollback.
- **Serving gaps.** A Qdrant collection missing at boot makes `Recommend` answer `UNAVAILABLE` until a
  restart; with no `recs:v1:popular` the row is empty; draft, deleted and sold-out listings are filtered
  only by the frontend's card hydration; cold start is a static popular list only.
- **No placement, no attribution, no online signal.** `Recommend` has only a `RecommendationContext` enum,
  returns no `request_id` and no `placement_id`, and the frontend logs no impression or click tied to the
  model that served it. There is no CTR, coverage or novelty per placement and model, and no way to compare
  a new model with the current one on real traffic (PROGRAM principle 1: no exposure log, no learning).
- **No observability.** No metric for model version or age, serving source or fallback, and no alert rule.

## What Changes

- **platform-core (proto, cross-service contract, all additive, `buf breaking` must pass)**:
  - Serving attribution fields `RecommendRequest.placement_id` (6), `RecommendResponse.request_id` (3) and
    `placement_id` (4) are **owned and added by C1** (`tracking-event-platform` task 1.4); this change does not
    edit them, it fills them in team-ai. An empty `placement_id` is derived from `context` (`HOMEPAGE` →
    `home.for_you`, `SIMILAR_ITEMS` → `pdp.similar`) so existing callers keep working.
  - `SearchListingsRequest.listing_ids` (10) for the eligibility check (9 is the in-flight hybrid `mode`;
    response fields 4/5 are C1's); lands after C1 1.4 (same file).
  - `AnalyticsQueryService.GetPlacementMetrics` (admin-only) for online evaluation; lands after C1's
    `analytics.proto` edits (same file).
  - A Prometheus rule file for the local stack and **ADR-0016** "Recsys model lifecycle: registry, promotion
    and online evaluation" (C1 = ADR-0014, C2 = ADR-0015, 0013 stays reserved for the placement engine).
- **platform-recsys**:
  - The trainer **declares** `platform-recsys/feature_views/als_interactions@v1.yaml` (C2's `fs-view.v1`
    schema) and, before every run, **triggers** a C2 build of that view (`BuildDataset`, scope
    `features.dataset`) and waits for it (`GetDatasetBuild`), then reads only that build. It refuses a build
    that fails or times out, whose schema version is unknown, whose feature versions differ from the view,
    whose watermark is too old or whose row count is too small. **Hard gate:** no code path, config or test fixture of the
    trainer reads `tracking_events` or any raw event table; a contract test fails CI if one appears.
  - Offline evaluation on a time-based holdout (Recall@K, NDCG@K, coverage, popularity baseline) and a
    **promotion gate**; a refused model is never visible.
  - **Generation publish**: per-version Qdrant collection, `recs:v2:*` generation keys, one atomic
    `current`/`previous` pointer flip after read-back verification, a run lock, `rollback`, and an optional
    **challenger** generation for an online A/B test by user hash.
  - Run report (status, reason, metrics, dataset build id and age).
- **team-ai** (placement serving):
  - A **minimal placement config** (the full placement engine / learned ranker of `plans/mlops` P2-T5/P3-T1
    stays later): three **placements** with per-placement strategy chains: `home.for_you` (ALS personal list,
    similar-to-recent items, trending, popular, floor), `pdp.similar` (item-item via Qdrant, co-visited,
    trending, popular, floor), `home.trending` (trending from online features, popular, floor). A no-op
    **ranker seam** that a later learned ranker fills by reading C2 features.
  - `Recommend` returns `request_id`, `model_version`, `placement_id`; reads the `recs:v2` pointer (and the
    challenger for users in the test bucket); **never errors** because a model, a feature or a store is
    missing; cold start uses C2 online features (`user.recent_items_24h`, `item.trending_1h`,
    `item.covisit_session_7d`, `user.event_count_30d`) through `FeatureService.GetOnlineFeatures` with the
    `features.read` service scope, behind a bounded timeout; a feature failure is recorded as
    `feature_failure`.
  - **Opt-out**: a request whose gateway-forwarded consent state (`x-client-tracking-consent`, C1) is `denied`
    is served as anonymous and non-personalized (no personal list, no user features, control arm).
  - **Eligibility** through team-search `listing_ids` (fail closed), catalog floor, Prometheus metrics,
    `RECS_BACKEND=memory` refused outside a local `ENVIRONMENT`.
- **team-analytics**: online evaluation from exposure logs (C1 impression/click events with attribution
  fields): CTR by `placement_id` and `model_version`, coverage and novelty, read through
  `GetPlacementMetrics` (also filterable by `request_id`). No feature or export code is added here by C3.
- **team-search**: `SearchListings` honours `listing_ids`; visibility and `in_stock` unchanged.
- **team-frontend**: passes `request_id`, `model_version`, `placement_id` and `position` from each
  `Recommend` response into the C1 SDK impression and click events for every recommendation row, and renders
  a `home.trending` row with the existing row component.
- **team-gateway**: re-vendored protos; routes `GetPlacementMetrics` and appends it to the admin-only
  procedure set after C1 task 3.10 (which moves the pinned set from {`QueryAuditLog`, `ReviewKyc`,
  `ResolveDispute`} to six); `Recommend` stays a pure forwarder.
- **platform-gitops**: (the analytics PVC and compose `DUCKDB_PATH` fix are C2's, not repeated here) Qdrant
  StatefulSet, the recsys CronJob wired to `valkey`, `qdrant` and the feature
  datasets in object storage, `team-ai` env (`RECS_*`, search and feature addresses), NetworkPolicies, and
  alert rules.
- **Root compose**: `team-ai` on `RECS_BACKEND=qdrant` + `v2`, a `recsys` profile trainer, and a local
  pipeline script: generate traffic → C1 events → C2 features and dataset → train → serve.
- **platform-e2e**: replace hand-seeded Redis with real runs; prove personalization (two users with
  different histories get different lists) and attribution (impression → click joined by `request_id`
  lands in analytics).
- **BREAKING (internal contract, not proto)**: Redis keys move from `recs:v1:*` flat keys to `recs:v2:*`
  generation keys; producer and consumer switch together (design Migration Plan).

## Capabilities

### New Capabilities
- `recsys-feature-consumption`: the trainer's declared feature view, consumption of C2 dataset builds by
  manifest with freshness/version checks and the hard no-raw-events gate, and serving's bounded read of C2
  online features.
- `recsys-model-lifecycle`: scheduled training, offline evaluation, promotion gate, atomic generation
  publish, rollback, challenger generation for online A/B, run reporting, local end-to-end run.
- `recsys-online-evaluation`: online metrics per placement and model version computed by analytics from
  exposure logs, their read API, and the comparison procedure for a challenger.

### Modified Capabilities
- `recommendations`: placements and their strategies, attribution fields in the contract and in the
  frontend's events, generation-aware serving that never errors, eligibility, feature-based cold start,
  trending row, metrics and alerts, and the trainer's input moving from the raw warehouse to the feature
  view.

## Impact

- Repos: `platform-core` (2 additive proto edits, rule file, ADR), `platform-recsys`, `team-ai`,
  `team-analytics` (online metrics only), `team-search`, `team-gateway`, `team-frontend`, `platform-gitops`,
  root compose + scripts, `platform-e2e` and the `FEATURES.yaml` of `team-ai`, `platform-recsys`,
  `team-analytics`, `team-frontend`.
- Program dependencies: C2 read contract (`BuildDataset`/`GetDatasetBuild` with `features.dataset`, dataset
  layout + manifest, `GetOnlineFeatures` with `features.read`) for the input switch and feature-based serving;
  C1 serving attribution proto fields (task 1.4, needed by the [now] serving work), attribution events and
  SDK for the attribution loop and online metrics, the forwarded consent header, and the admin-set task. Lifecycle, pointer, gate, eligibility and fallbacks start now (tasks mark each).
- Architecture rules: Rule 1 (frontend calls only the gateway; it logs events through the C1 SDK →
  `/api/track`), Rule 2 (gateway forwards; the only edge policy change is the admin set), Rule 3 (team-ai
  asks team-search and team-analytics over gRPC; the trainer reads only C2 datasets in object storage,
  never a service DB; model outputs stay in the `recs:v2` namespace), Rule 4 (all contract changes in
  platform-core), Rule 5 (no new topic or queue).
- Runtime: up to one eligibility call, one floor call and one feature call per `Recommend`, each bounded;
  nightly Spark run trains twice (holdout fit + full fit).
- Privacy: the response carries listing ids, scores, ranks and provenance only; online metrics are
  aggregates; personal lists are written for logged-in users only; opted-out users are excluded from training
  data by C2 and served non-personalized results here.

## Non-goals

- No learned ranker / LTR, no LLM reranker, no two-tower retrieval, no blend algebra or `EXPLAIN` of the
  full placement engine (`plans/mlops` P2-T5/P3-T1/P3-T3 stay later); only the ranker seam is added.
- No feature definitions, materializers, dataset builder, online store or event taxonomy here: those are
  C2 and C1. C3 only declares a view and reads.
- No interleaving or shadow traffic, no automatic online promotion; a challenger is promoted by an
  operator reading the online metrics.
- No MLflow or model-registry service; the registry is the run report plus the `recs:v2` pointers.
- No exclusion of already-bought items, no purchase-weighted training beyond what the view's feature
  provides.
- No new placements beyond `home.for_you`, `pdp.similar`, `home.trending` (search, cart and assistant
  placements are C1-tracked surfaces but not served by this change).
