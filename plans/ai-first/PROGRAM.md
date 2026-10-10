# Program — AI-first marketplace (tracking → feature store → models → placements)

Status: draft 2026-10-01 (planning only), reconciled across C1/C2/C3 on 2026-10-01. Owner of this file: root
workspace. Open user decisions for all three changes: `DECISIONS.md` (same folder).
This brief is the shared contract the three OpenSpec changes below are written against.
If a change needs to deviate, it updates this file in the same edit.

## Why

The site exists to serve AI and recommendations. Today:

- Tracking is thin: `TrackingEvent` has 4 event types (`VIEW`, `CLICK`, `ADD_TO_CART`,
  `IMPRESSION`), no attribution (which placement / request / model served an item), and no
  server-side facts (orders, favorites, reviews, follows) reach `team-analytics`.
- There is no feature layer: the recommender would read a bespoke raw-event export, so every
  new model or tweak re-implements feature logic (training/serving skew, duplication, leakage).
- `plans/mlops/` already lists the missing pieces (attribution P1-T1, eval P1-T2, registry
  P1-T4, nearline P2-T4, feature store P3-T2) but places the feature store late, in a new repo,
  and lets `platform-recsys` own nearline signals. This program moves the data foundation first.

## Principles

1. **Every surface is a placement.** Anything that shows a ranked list (home rows, search
   results, PDP "similar", recommendation rows, AI assistant suggestions) has a stable
   `placement_id`, returns a `request_id` and `model_version`, and the client logs impressions
   and clicks with those three fields plus `position`. No exposure log → no learning, no eval.
2. **`team-analytics` owns behavioral data end to end**: event taxonomy and schema, ingestion,
   identity stitching, privacy/retention/erasure, data quality, and the **feature store**
   (registry, offline store, online store, materializers, read contract).
3. **Consumers read features, never raw events.** A model declares a *feature view* (the list
   of `feature@version` it needs), pins versions, and receives exactly those. Tuning = edit the
   view or add a feature version; no consumer code in `team-analytics`, no feature logic in the
   consumer. Feature references are `<entity>.<name>@v<N>`.
4. **One definition, two materializations.** Each feature is one SQL definition in the
   registry. The offline store and the online store are both produced from that same SQL
   (batch for history, micro-batch over the recent window for online). A parity test fails CI
   when the two disagree. Training must go through the point-in-time API (hard gate): a feature
   store wired on one side only is worse than none (plans/mlops P3-T2 lesson).
5. **Features ≠ model outputs.** Features live in the feature store (`team-analytics`). Model
   artifacts and outputs (ALS vectors, precomputed lists, version pointer) are owned by the
   model's producer (`platform-recsys`) under its own namespace and registry.
6. **Heavy, first-party, privacy-bounded tracking.** Track a lot, but: first-party only,
   random anonymous ids (no fingerprinting), IP never stored in events, PII classes per field,
   retention windows, erasure path, opt-out honoured at the edge **and downstream**: an opted-out subject
   is excluded from personalization features and datasets (C2) and served non-personalized (C3); business
   facts (orders, favorites) still flow.
7. **Contracts, not shared databases** (AGENTS.md §3): proto in `platform-core`; domain facts via
   each service's outbox to its own `<domain>.events` topic; nobody reads another service's DB.

## Architecture

```
browser (team-frontend tracking SDK)
  └─ POST /api/track ─► team-gateway (validate, cap, rate-limit, consent, strip IP;
                          │   forwards x-client-tracking-consent on every upstream call)
                          └─► analytics.events ─┐
services (outbox) ─► payment.events, order.events, engagement.events, listing.events ─┤
                                                                                       ▼
team-analytics: ingest → events tables (DuckDB, PVC) → data-quality checks → retention / erasure
   │  (AnalyticsOpsService: InspectEvents, EraseSubject, GetDataQualityReport — admin)
   ├─ feature registry (features/registry.yaml)
   ├─ batch materializer   → offline store: object storage Parquet (versioned, manifest)
   ├─ micro-batch materializer (same SQL, recent window) → online store: valkey/Redis `fs:`
   ├─ dataset builder: point-in-time training sets for a declared view (snapshot spines now,
   │  events spines with ASOF JOIN for rankers later)
   ├─ FeatureService gRPC: GetOnlineFeatures, DescribeFeatures (features.read);
   │                       BuildDataset, GetDatasetBuild (features.dataset)
   └─ AnalyticsQueryService.GetPlacementMetrics (online evaluation, admin)
        │ BuildDataset → GetDatasetBuild        │ GetOnlineFeatures
        ▼ (training datasets)                   ▼ (online features)
platform-recsys trainer (declares views) ── model outputs: Qdrant + `recs:v2:*` + pointer
                                                            │
team-ai placement serving (Recommend, minimal placements.yaml): model outputs + online features
   + eligibility (team-search) └─ returns request_id, model_version, placement_id → frontend logs
   impressions/clicks
```

## Shared contracts (names are binding across the three changes)

### Events (owned by `tracking-event-platform`)
- Client events on `analytics.events` (additive `TrackingEvent` fields; new `EventType` values):
  `PAGE_VIEW`, `VIEW` (listing detail; add `dwell_ms`), `IMPRESSION`, `CLICK`, `SEARCH`
  (query, filters, `result_count`), `ADD_TO_CART`, `REMOVE_FROM_CART`, `CHECKOUT_START`,
  `SHARE`, `IDENTIFY` (anonymous → user stitching at login), `CONSENT_UPDATE` (opt-out / opt-in;
  latest state per subject exposed as `consent_state_latest` for C2).
- Attribution fields on every list-scoped event: `placement_id`, `request_id`, `model_version`,
  `position`.
- Server-side facts, each from its owner's outbox to its own topic, consumed by analytics:
  `payment.events` (exists: `PaymentSettled`), `order.events` (new: placed / cancelled / shipped),
  `engagement.events` (new: favorite added/removed, review created, follow/unfollow).
- Placement id format: `<surface>.<slot>`, e.g. `home.for_you`, `home.trending`, `pdp.similar`,
  `search.results`, `assistant.suggestions`.
- Serving attribution proto fields are **owned by C1** (task 1.4): `RecommendRequest.placement_id = 6`,
  `RecommendResponse.request_id = 3`, `placement_id = 4`, `SearchListingsResponse.request_id = 4`,
  `model_version = 5`. C3 and team-search only fill them. Other numbers in flight: `SearchListingsRequest.mode
  = 9` (hybrid change), `SearchListingsRequest.listing_ids = 10` (C3). No collision.
- Consent forwarded by the gateway as metadata `x-client-tracking-consent: granted|denied` (rebuilt by the
  gateway, never trusted from the client).
- Raw-event retention (C1 owns it for the program): behavioural rows 13 months; free text (search query,
  path, referrer) nulled after 90 days; facts 25 months; quarantine 14 days.

### Features (owned by `analytics-feature-store`)
- Reference format: `<entity>.<name>@v<N>` (e.g. `item.views_7d@v1`); views `<view>@v<N>`; entities:
  `user` (principal id; anonymous visitors as `anon:<anonymous_id>`), `item` (`listing_id`), `user_item`,
  `session`.
- Offline layout: `s3://<bucket>/features/offline/<entity>.<name>@v<N>/dt=YYYY-MM-DD/*.parquet` +
  `_manifest.json` (schema_version, watermark, row count); datasets:
  `s3://<bucket>/features/datasets/<view>@v<N>/<build_id>/` + manifest.
- Online keys: `fs:v1:<entity>:<id>` hash, field `<name>@v<N>`, per-feature TTL. Online freshness promise
  **~2 min** (60 s micro-batch of the same SQL). This replaces `plans/mlops` P2-T4's "event → Redis < 10 s":
  seconds would need a second (streaming) implementation of every window, i.e. the training/serving skew
  principle 4 forbids, and no C3 consumer needs seconds.
- Read contract: `platform.analytics.v1.FeatureService`: `GetOnlineFeatures`, `DescribeFeatures` (scope
  `features.read`, held by `service-team-ai`) and `BuildDataset`, `GetDatasetBuild` (scope
  `features.dataset`, held by `service-platform-recsys`); both scopes service-only, granted to no role; not
  routed by the gateway. Client failure reason: `feature_failure`.
- Datasets: requested by a consumer-owned view file (`platform-recsys/feature_views/als_interactions@v1.yaml`,
  schema `fs-view.v1`, written by C3); v1 builds **snapshot spines** (feature values computed at a list of as-of
  times, e.g. `[watermark - 2d, watermark]` for ALS holdout + publish); events spines (ASOF JOIN on label
  times) exist for later rankers. The snapshot `watermark` is the build time capped by the ingestion watermark.
  A trainer triggers `BuildDataset` and waits with `GetDatasetBuild` before each run.
- Consent: subjects whose latest `CONSENT_UPDATE` is `denied` are excluded from `user`/`user_item`/`session`
  features and from dataset rows; item-level features keep counting business facts.
- Feature-store retention: offline partitions 180 d; datasets 30 d (180 d when pinned by a promoted model);
  raw events follow C1.
- Initial features (recommender needs): `user_item.implicit_score_decayed`, `item.views_{1d,7d,30d}`,
  `item.clicks_7d`, `item.ctr_7d_position_debiased`, `item.add_to_cart_7d`, `item.orders_30d`,
  `item.favorites_30d`, `item.trending_1h`, `user.recent_items_24h`, `user.event_count_30d`
  (cold-start flag), `item.covisit_session_7d` (top-N co-viewed).

### Models and serving (owned by `recommendations-end-to-end`)
- Trainer reads only a declared view, through a build it triggers and waits for; never `tracking_events`.
- Model outputs + pointer: `recs:v2:*` (platform-recsys namespace), per-version Qdrant collection.
- Serving: `RecommendationService.Recommend` fills `request_id`, `model_version`, `placement_id` (fields from
  C1). Placements in C3 are a **minimal config** (`team-ai/app/configs/placements.yaml`: `home.for_you`,
  `pdp.similar`, `home.trending` with strategy chains and an identity ranker seam); the placement engine and
  learned ranker stay later. An opted-out request is served non-personalized.
- Online evaluation: `AnalyticsQueryService.GetPlacementMetrics` (admin-only) over C1 exposure logs.

### Admin-only gateway procedures
Pinned set today: {`AuditService/QueryAuditLog`, `VerificationService/ReviewKyc`,
`EngagementService/ResolveDispute`}. C1 task 3.10 adds `AnalyticsOpsService/{InspectEvents, EraseSubject,
GetDataQualityReport}` and updates AGENTS.md §4 + the pinned test (6); C3 task 3.2 appends
`AnalyticsQueryService/GetPlacementMetrics` (7). `FeatureService` is internal-only (gateway answers 501).

### ADR numbers
0013 reserved (placement engine, later). **C1 = ADR-0014** "Tracking event platform"; **C2 = ADR-0015**
"Feature store in team-analytics"; **C3 = ADR-0016** "Recsys model lifecycle: registry, promotion and online
evaluation" (supersedes the ADR-0014 named in `plans/mlops` P1-T4).

### Ownership resolutions (single owner per artifact)
| Artifact | Owner (task) | Others |
|---|---|---|
| Serving attribution proto fields | C1 1.4 | C3 fills them; never edits |
| `analytics.proto` | C1 1.1, 1.2 first | C3 1.3 (`GetPlacementMetrics`) after C1 1.2 |
| `search.proto` | C1 1.4 (response 4, 5) | C3 1.2 (`listing_ids = 10`) after it; hybrid `mode = 9` |
| Admin-only set + AGENTS.md §4 | C1 3.10 | C3 3.2 appends |
| Consent forwarding header | C1 3.11 | C3 5.11 reads it |
| `consent_state_latest` | C1 5.5 | C2 2.3 / 8.8 read it |
| Raw-event retention | C1 6.2 | C2 never sweeps raw rows |
| Raw-row erasure (`EraseSubject`) | C1 6.3 | calls C2's `Erase` hook (C2 8.4) |
| `ingested_at` column | C2 3.1 | C1 does not re-add it |
| Compose `DUCKDB_PATH` `/tmp` fix | C2 9.1 | C1 11.1 needs it |
| Analytics PVC (gitops) | C2 10.1 | C1 10.2 needs it |
| Service-only scopes `features.read` / `features.dataset` | C2 4.1 | C3 4.10 / 5.9 use them |
| `plans/mlops/INDEX.md` edit | C2 14.1 | C1 12.3, C3 1.5 add pointer lines in their own task files |
| View file `als_interactions@v1.yaml` | C3 4.1 | C2 12.1 validates it |
| Dataset client `recsys/datasets.py`, raw-read guard test | C2 12.2, 12.3 | C3 4.10 uses / extends |
| `tracking` main spec | C1 (contract, collector requirements) | C2 (WarehouseWriter requirement only); disjoint; **C1 archives first**, C2 rebases |

## Changes and sequencing

| # | OpenSpec change | Depends on | Can start |
|---|---|---|---|
| C1 | `tracking-event-platform` | C2 9.1, 10.1 (small, dependency-free) for compose/gitops only | now; lands first |
| C2 | `analytics-feature-store` | C1 contracts (field names), `consent_state_latest`, erasure hook; runs on today's 4 events first | now, in parallel |
| C3 | `recommendations-end-to-end` (revised) | C1 1.4 (proto fields) for serving; C2 read contract for the input switch; C1 attribution for online eval | lifecycle/serving parts now; input switch after C2 |

Archive order: C1, then C2 (both modify `tracking`), then C3.

## Cross-change dependency graph

`A x.y → B u.v` means task u.v of change B needs task x.y of change A. Within-change edges are in each
`tasks.md`.

**C1 → C3**
- C1 1.4 (serving proto fields) → C3 1.1, 1.2, 3.1, 5.4, 9.1
- C1 1.2 (`AnalyticsOpsService` in `analytics.proto`) → C3 1.3
- C1 3.10 (admin set) → C3 3.2
- C1 3.11 (consent header) → C3 10.6 (opt-out scenario; C3 5.11 builds against the header now)
- C1 4.2 (SDK impressions) → C3 9.2, 10.9
- C1 5.1 (attribution columns stored) → C3 6.1, 10.7, 10.9

**C1 → C2**
- C1 5.5 (`consent_state_latest`) → C2 8.8, 15.8 (C2 2.3 ships a stub until then)
- C1 6.3 (`EraseSubject`) → C2 8.4 (hook wiring)
- C1 9.4 (`GetDataQualityReport`) → C2 8.2 (real verdict; defaults to pass until then)

**C2 → C1**
- C2 9.1 (compose `DUCKDB_PATH`) → C1 11.1
- C2 10.1 (analytics PVC / Deployment) → C1 10.2

**C2 → C3**
- C2 1.1, 4.1, 6.1, 6.3 (proto, scopes, read path, authz) → C3 5.9
- C2 4.1, 5.5, 6.3, 12.2, 12.3 (scopes, build RPCs, authz, dataset client, guard) → C3 4.10 (→ 7.3, 8.3,
  10.3, 10.8)
- C2 10.2, 10.3 (NetworkPolicy, MinIO users) → C3 8.3

**C3 → C2**
- C3 4.1 (view file) → C2 12.1 (→ C2 9.3, 15.6)

No cycles: C1 1.4, C2 9.1/10.1 and C3 4.1 have no cross-change dependency and should land first.

Later (not in these changes; re-point `plans/mlops` to the feature store): placement engine /
ranker (P2-T5, P3-T1; ADR-0013), two-tower (P3-T3), drift monitoring (P3-T4), search learning-to-rank,
AI-assistant personalization, demand forecasting (ADR-0012) as feature-store consumers.

## Supersedes in plans/mlops
- P1-T1 attribution → C1 (`config_version` dropped). P2-T4 nearline → C2 (owned by `team-analytics`, not
  `platform-recsys`; ~2 min freshness instead of < 10 s).
- P3-T2 feature store → C2 (inside `team-analytics`, not a new `platform-featurestore` repo).
- P1-T2 eval + P1-T4 registry/gate → C3, under ADR-0016 (not the ADR-0014 named in P1-T4).
- The `plans/mlops/INDEX.md` rows are updated once, by C2 task 14.1.
