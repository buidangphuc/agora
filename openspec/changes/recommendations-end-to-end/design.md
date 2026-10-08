## Decisions needed from the user

Merged across C1/C2/C3 (deduplicated, with the legal-review items): `plans/ai-first/DECISIONS.md`.

Each item has a recommended default; `tasks.md` is written against the defaults. Changing one changes the
named tasks only. Names shared with C1/C2 follow `plans/ai-first/PROGRAM.md` and are not decisions here.

| # | Decision | Recommended default | Alternative | Tasks affected |
|---|---|---|---|---|
| U1 | Eligibility (published + in-stock) source | **team-ai asks team-search** with additive `SearchListingsRequest.listing_ids` (D7) | A new internal RPC on team-search; or team-ai keeps its own listing set from `listing.events` | 1.2, 2.x, 5.5, 5.6 |
| U2 | Eligibility failure mode | **Fail closed** (no items; the row hides) | Fail open and rely on frontend hydration | 5.6 |
| U3 | Gate thresholds (deployed / local) | Recall@10 ≥ 0.01 / 0.05; lift over popularity ≥ 1.0 / 1.0; users ≥ 50 / 4; items ≥ 20 / 8; coverage ≥ 0.05 / 0.3; mean list overlap ≤ 0.9 / 0.8 | Stricter once real traffic exists | 4.4, 7.2, 8.3 |
| U4 | Evaluation cost | **Two fits per run** (fit at the holdout cutoff for metrics, fit at the watermark for publish) | Publish the holdout fit (one fit, loses the newest interactions) | 4.6 |
| U5 | Cadence | Train **nightly 03:00 UTC**; each run **triggers** a C2 build (`BuildDataset`) and waits up to `DATASET_BUILD_TIMEOUT_SECONDS` (1800) via `GetDatasetBuild`; refuse the build when it fails, times out, or its `watermark` (C2: build time capped by the ingestion watermark) is older than **3h** at completion (i.e. ingestion is stalled); staleness alert at **36h** | Train every 6h (same trigger, more build load) | 1.4, 4.1, 4.10, 8.3, 8.4 |
| U6 | ADR number | **Resolved by the program planner: ADR-0016** "Recsys model lifecycle: registry, promotion and online evaluation" (absorbs `plans/mlops` P1-T2/P1-T4, whose ADR-0014 reference is superseded; C1 = ADR-0014, C2 = ADR-0015; 0013 stays reserved for the full placement engine) | — | 1.5 |
| U7 | Personal lists for anonymous visitors | **Not written** (anonymous interactions still train item vectors; anonymous visitors get similar-to-recent from features) | Write `anon:` lists for future anonymous personalization | 4.5 |
| U8 | Online comparison of a new model | **A/B by user hash with a challenger generation** (D12): default share **10 %**, sticky per user, operator promotes after reading CTR by `model_version`; 0 % = off | Shadow scoring (no exposure, so no outcome signal); team-draft interleaving (most sensitive, but needs per-item arm credit that the PROGRAM attribution fields do not carry) | 4.8, 5.2, 10.6, 10.7 |
| U9 | Placement list | **`home.for_you`, `pdp.similar`, `home.trending`**; `home.trending` rendered as a second home row with the existing row component | Serve `home.trending` API-only (no UI change); or add `cart.cross_sell` | 5.4, 9.3, 10.6 |
| U10 | Exposure logging ownership | **Client-side**: team-frontend logs `IMPRESSION`/`CLICK` with the four attribution fields through the C1 SDK (visibility rule owned by C1); team-ai emits no served-event (trace span + metric only); team-analytics owns storage and metrics | team-ai also emits a server-side "served" event (impression-loss measurable, but a second producer on `analytics.events`, which C1 reserves for the gateway) | 9.2, 10.9 |
| U11 | Online-metrics read surface | **`AnalyticsQueryService.GetPlacementMetrics`, admin-only** (appended to the admin-only set after C1 3.10 grows it from 3 to 6, so 7) | Prometheus gauges exported by team-analytics (no RPC, no admin-set change, but no `request_id` drill-down for the attribution proof) | 1.3, 3.2, 6.2 |
| U12 | Trending under the C2 contract (no top-N-by-feature read) | **Re-score a candidate pool** (current generation's popular list ∪ newest eligible listings) by `item.trending_1h`, cached in-process 30 s (D9) | Ask C2 for a ranked "top items by feature" read (a PROGRAM contract change) | 5.7 |

## Context

See `proposal.md` (Why) for the verified gaps. State relevant to the design:

- **Program.** C1 (`tracking-event-platform`) defines events and attribution fields (`placement_id`,
  `request_id`, `model_version`, `position`) and the frontend SDK. C2 (`analytics-feature-store`) owns
  features in `team-analytics`: offline layout `s3://<bucket>/features/offline/...`, datasets
  `s3://<bucket>/features/datasets/<view>@v<N>/<build_id>/` + manifest (schema_version, watermark, row
  count), online keys `fs:v1:<entity>:<id>`, and `platform.analytics.v1.FeatureService`
  (`GetOnlineFeatures`, `DescribeFeatures` gated by the `features.read` service scope; `BuildDataset`,
  `GetDatasetBuild` gated by `features.dataset`). Feature references are `<entity>.<name>@v<N>`. Entities: `user`
  (principal id, anonymous as `anon:<anonymous_id>`), `item` (`listing_id`), `user_item`, `session`.
- **Trainer** (`platform-recsys`) today: reads Parquet or BigQuery `tracking_events`, builds triples, fits
  ALS, writes fixed Qdrant collections (pruning other generations first) and flat `recs:v1:*` keys, then
  sets `recs:v1:model_version`. No evaluation, no gate.
- **Serving** (`team-ai`): `PrecomputedCache` (`recs:v1:user:{id}`, `recs:v1:popular`), `ModelVersionReader`
  (30 s cache), `QdrantRetrievalBackend` checks the collection once at boot. Strategy is picked from
  `RecommendationContext`. Identity binding (`_bound_identity`) is correct and unchanged.
- **Frontend**: `RecommendationsRow` (server component) calls `getRecommendations` with `HOMEPAGE` or
  `SIMILAR_ITEMS` and renders `ListingGrid`; it logs nothing.
- **Search**: indexes `status`, `stock`; default `status=published`; `in_stock=true` filter;
  `SearchListings` requires `search:read`. In flight: `add-hybrid-retrieval-platform` takes field 9.
- **Deployed**: no Qdrant, Redis is `valkey`, team-ai has `REDIS_ENABLED=false`, no Prometheus rule file.

## Goals / Non-Goals

**Goals:** a model trained only from a declared feature view reaches users only after it is checked; a bad
run is invisible; one command rolls back; every placement answers (never errors) and never returns an
ineligible listing; every response is attributable; analytics measures each placement and model online; a
new model can be compared on real traffic; the whole loop runs locally with the same code as in-cluster.

**Non-Goals (design-level):** no change to the ALS algorithm, weights (now the C2 feature) or
hyperparameter defaults; no change to identity binding; no Spark cluster; no learned ranker.

## Decisions

### D1 — Input: a declared feature view and C2 dataset builds (replaces the bespoke export)

`platform-recsys/feature_views/als_interactions@v1.yaml` (consumer-owned and written by this change, task 4.1, in
C2's `fs-view.v1` schema; C2 12.1 only validates it against the registry):

```yaml
view: als_interactions
version: 1
entity: user_item                      # keys: user (principal id or anon:<id>), item (listing_id)
features: [user_item.implicit_score_decayed@v1]   # ALS confidence weight
labels: []
spine: {kind: snapshot, as_of: [watermark - 2d, watermark]}   # holdout_start, watermark (D5)
window_days: 30                          # interaction history bound
consumer: platform-recsys
```

The trainer's only input driver is `INPUT_DRIVER=feature_dataset`, built on C2's client `recsys/datasets.py`
(C2 task 12.2): each run calls `request_build(view)` (`BuildDataset` as `service-platform-recsys` with
`features.dataset`), then `wait(build_id, DATASET_BUILD_TIMEOUT_SECONDS)` (polls `GetDatasetBuild`), then
`load(manifest)` of **that** build (C2 checks `view_hash`, checksums, `invalidated`). It never falls back to an
older build. It refuses — with a typed reason in the run report — a build that ends FAILED (`dataset build
failed`) or does not finish in time (`dataset build timeout`), a manifest whose `schema_version` is unknown,
whose feature list or versions differ from the view file, whose `watermark` is older than
`MAX_DATASET_AGE_SECONDS` (U5) at completion (C2 caps the snapshot watermark by the ingestion watermark, so this
catches stalled ingestion), or whose row count is below `MIN_DATASET_ROWS`. Opted-out subjects are already
absent from the build (C2). It reads the Parquet parts
with `pyarrow.fs.S3FileSystem` and asserts the read row count equals the manifest. Triples are
`(user_key, listing_id, implicit_score_decayed)`; rows whose `user_key` starts with `anon:` train item
vectors but get no personal list (U7).

**Hard gate (PROGRAM principle 3):** the `parquet`/`bigquery` warehouse drivers and `WAREHOUSE_*` config are
removed; a `fixture` driver reading a local directory in the same dataset layout exists only for tests and
is refused unless `ENV` is `test`/`local`; `tests/test_no_raw_events.py` fails if any file under `recsys/`,
`feature_views/` or the job's config mentions `tracking_events`, `analytics.events` or a warehouse DSN.

*Before C2 lands*, all trainer work runs against hand-built fixtures in the PROGRAM dataset layout behind the
same `DatasetSource` interface (a fake "build" that returns a fixture manifest), so the switch to real builds is
configuration, not trainer code. The raw-read guard test is C2's (12.3); task 4.10 extends its scope.
*Alternatives:* the previous bespoke `team-analytics` snapshot export (dropped: duplicates the feature store,
re-implements feature logic in the consumer, and is exactly the skew PROGRAM principle 4 forbids); reading
the offline feature tables directly (bypasses the point-in-time builder — the P3-T2 lesson).

### D2 — Scheduling and the run lock

Deployed: the `platform-gitops` CronJob (`concurrencyPolicy: Forbid`, `backoffLimit: 1`) at 03:00 UTC.
Local: a `recsys-train` compose service under profile `recsys` (`restart: "no"`). Both take the Redis lock
`recs:v2:lock` (`SET NX EX`, TTL = max run time) and exit `skipped` if it is held.

### D3 — Qdrant: one collection per generation, created by the trainer

`platform-gitops` gets a Qdrant StatefulSet (`v1.12.4`), PVC, Service `qdrant`, NetworkPolicy. Each
generation writes `{QDRANT_ITEM_COLLECTION}__{mv}` (and the user collection likewise) with `size = ALS_RANK`,
`Cosine`, and the existing payload indexes. team-ai never creates collections; it derives the name from the
generation it serves (D4). Publishing refuses an `ALS_RANK` different from the current generation's
`expected_dim` unless `ALLOW_DIM_CHANGE=true`.
*Alternatives:* Qdrant aliases (atomic in Qdrant but not jointly with Redis); in-place upsert + prune (today:
mixed generations, nothing to roll back to).

### D4 — Redis key contract v2 and the pointers

| key | value |
|---|---|
| `recs:v2:gen:{mv}:user:{user_key}` | ranked `[{listing_id, score}]` (logged-in users only, U7) |
| `recs:v2:gen:{mv}:item:{listing_id}` | similar items, same shape |
| `recs:v2:gen:{mv}:popular` | popularity list, same shape |
| `recs:v2:gen:{mv}:meta` | hash: `published_at`, `dataset_build_id`, `dataset_watermark`, `feature_view`, `items`, `users`, `expected_dim`, `metrics` (JSON), `kind` (`als`/`popularity`), `family` |
| `recs:v2:current` | `{mv}` — what serving follows for everyone outside the test bucket |
| `recs:v2:previous` | `{mv}` — rollback target |
| `recs:v2:challenger` | `{mv}` — optional, served to the test bucket only (D12) |
| `recs:v2:challenger:share` | integer percent 0–50 |
| `recs:v2:run:last` | JSON run report |
| `recs:v2:lock` | run lock (D2) |

`model_version` = `{family}-{UTC yyyymmddTHHMMSSZ}`; `family` names the model configuration (default
`als_v1`; a hyperparameter or view-version change is a new family). Publish order: write generation keys
(TTL `RECS_CACHE_TTL_SECONDS`, default 7 days) and collections → read back counts → `MULTI; SET previous
<current>; SET current <mv>; EXEC` (or `SET challenger <mv>` in challenger mode) → delete generations not in
{current, previous, challenger} → write the run report. Every artifact name derives from one pointer value,
so a response never mixes generations; a user absent from the new generation misses (cold start). Rollback
is the swapped `MULTI` (`python -m recsys rollback`).
*Alternatives:* `RENAME` from staging inside `MULTI` (O(keys) blocking); one hash per generation (no
per-user TTL); flat keys + version flip (today's mixed-generation bug).

### D5 — Evaluation and promotion gate

Each dataset build carries two as-of snapshots: `holdout_start` = watermark − `EVAL_HOLDOUT` (default the
latest 10 % of the window, capped at 2 days) and `watermark`. Fit 1 trains on the `holdout_start` snapshot.
Holdout positives for a user are the items present for that user at `watermark` and absent at
`holdout_start` — new interactions, which is also why ranking excludes the user's train-seen items. Metrics:
Recall@K, NDCG@K (`EVAL_K`=10), catalog coverage of top-K lists, popularity baseline Recall@K (popularity
from the `holdout_start` snapshot). Fit 2 trains on the `watermark` snapshot and is the published one (U4).
Gate checks (U3) run on fit-1 metrics and fit-2 artifacts (counts, finite vectors, mean pairwise Jaccard of
top-K over ≤500 sampled user pairs). Report JSON to `s3://<models bucket>/models/{mv}/report.json`, summary
in `gen:{mv}:meta` and `run:last`. Refused with no `current` ⇒ publish a `kind=popularity` generation so a
floor exists.
*Alternative:* gate against the current model on the same holdout (P1-T4) — later; the fixed floor plus
popularity lift stops broken models now, and online A/B (D12) covers "better than current".

### D6 — Backend per environment

| env | `RECS_BACKEND` | data |
|---|---|---|
| unit tests | `memory` | fixtures |
| local compose | `qdrant` | real Qdrant; empty until the first run, which serving tolerates (D8) |
| e2e overlay | `qdrant` | real runs; `RECS_MODEL_VERSION_CACHE_SECONDS=0` |
| kind / staging / prod | `qdrant`, set explicitly in `envs/services/team-ai.yaml` | CronJob output |

`memory` is refused when `ENVIRONMENT` is not local (same rule as `GRPC_BEARER_FALLBACK_ENABLED`); since
gitops leaves team-ai's `ENVIRONMENT` unset, the explicit `RECS_BACKEND=qdrant` is the real control.

### D7 — Eligibility through team-search

`SearchListingsRequest.listing_ids = 10` restricts the match (`terms` on `id`) on top of the default
`status=published`; at most 200 ids (`INVALID_ARGUMENT` above). team-ai calls it once per request with up to
`RECS_CANDIDATE_TOP_K` ids, `in_stock=true`, as `service-team-ai` with `search:read`, keeping candidates in
candidate order. The same client serves the catalog floor (empty query, `in_stock=true`, newest, cached
`RECS_FLOOR_CACHE_SECONDS`=60). Timeout `RECS_ELIGIBILITY_TIMEOUT_MS`=50; error or timeout ⇒ empty items
(U2). Frontend hydration stays as defence in depth.
*Alternatives:* see U1; filtering at train time is a cadence stale.

### D8 — Placements, strategy chains and the ranker seam

`team-ai/app/configs/placements.yaml` (minimal placement config; the placement engine of P2-T5 and the learned
ranker stay later), validated at startup (unknown strategy or placement ⇒ boot fails):

| placement | chain (stop when `RECS_CANDIDATE_TOP_K` candidates are collected) |
|---|---|
| `home.for_you` | `als_user` → `similar_to_recent` → `trending` → `popular` → `floor` |
| `pdp.similar` | `item_similar` (needs seed) → `covisit` → `trending` → `popular` → `floor` |
| `home.trending` | `trending` → `popular` → `floor` |

Strategies: `als_user` = generation personal list; `item_similar` = ANN on the seed in
`{collection}__{mv}`; `similar_to_recent` = ANN on up to 5 items of `user.recent_items_24h` (one batched
Qdrant query, merged by max score); `covisit` = `item.covisit_session_7d` of the seed; `trending` = D9;
`popular` = generation popular list; `floor` = D7 floor. A strategy that cannot run (no generation, no seed,
feature unavailable, store down) yields `[]` and the chain continues — never an error. Pipeline per request:
`CONTEXT → RETRIEVE (chain) → FILTER (dedupe, drop seed, eligibility) → RANK → TRUNCATE → FLOOR (if empty)`.
`RANK` calls a `Ranker` protocol `rank(placement, context, candidates, features) -> candidates`; the only
implementation is `identity` (keep chain order), selected by `ranker: identity` per placement. A later
learned ranker reads C2 features through the same `FeatureClient` and is a config switch.
Placement resolution: `placement_id` if set (unknown ⇒ `INVALID_ARGUMENT`), else from `context`
(`HOMEPAGE`→`home.for_you`, `SIMILAR_ITEMS`→`pdp.similar`, `UNSPECIFIED` with seed→`pdp.similar`,
otherwise `home.for_you`).
**Opt-out:** when the gateway-forwarded metadata `x-client-tracking-consent` (C1 task 3.11) is `denied`, the
request is served as an anonymous caller with no anonymous id: `user_id`/`anonymous_id`/principal user are not used
for personalization, strategies `als_user` and `similar_to_recent` are skipped, no `user:` feature is requested,
the arm is control (current generation); seed-based (`item_similar`, `covisit`), `trending`, `popular` and `floor`
still run. Absent header (pre-C1) ⇒ treated as granted. Caller binding (who may ask for whom) is unchanged. C2
independently removes the subject from features and training data, so this is the immediate path and C2 the
durable one. The response `source` of the first strategy that contributed is recorded in
metrics, not in the proto.
*Alternative:* the full P2-T5 engine (pools, blend, `EXPLAIN`) — later; this keeps exactly what three
placements use.

### D9 — Online features in serving (C2 read contract)

`FeatureClient` wraps `FeatureService.GetOnlineFeatures` (service principal `service-team-ai`, scope
`features.read`, timeout `RECS_FEATURES_TIMEOUT_MS`=30). One call per request batches the entities the
placement needs: `user:<id>` (`user.recent_items_24h@v1`, `user.event_count_30d@v1`) and, for
`pdp.similar`, `item:<seed>` (`item.covisit_session_7d@v1`). Feature versions are pinned in
`placements.yaml`. `trending` does not call per request: every `RECS_TRENDING_CACHE_SECONDS` (30) a
background refresh scores a pool (current popular list ∪ cached newest eligible listings, ≤200 ids) by
`item.trending_1h@v1` and keeps the sorted list in-process (U12). A feature failure is recorded as
`feature_failure` (distinct from `no_history`; same term as C2's client) and the chain continues. Before C2 ships, `FeatureClient` is a
`NullFeatureClient` (always "unavailable"), so every chain already degrades correctly.

### D10 — Attribution in the contract and the frontend

team-ai generates `request_id` (UUIDv7) per `Recommend` and returns it with `placement_id` and
`model_version` (the generation actually served: current, challenger, or `fallback` constant when only
the floor served). `RecommendedItem.rank` is the `position`. team-frontend passes these into the C1 SDK:
one `IMPRESSION` per visible card (visibility rule from C1) and a `CLICK` on navigation, each with
`placement_id`, `request_id`, `model_version`, `position` (U10). `RecommendationsRow` takes a `placementId`
prop; the home page renders `home.for_you` and `home.trending`, the PDP `pdp.similar`.

### D11 — Online evaluation in team-analytics

From C1 events in the warehouse, over a window and grouped by `placement_id` × `model_version` (and
`family` = `model_version` up to its last `-`):
- `impressions` = distinct (`request_id`, `listing_id`) impressions; `clicks` = distinct clicks whose
  (`request_id`, `listing_id`) has an impression; `ctr` = clicks / impressions.
- `coverage` = distinct impressed listings / distinct published listings seen by analytics in the window.
- `novelty` = mean over impressions of −log2(p(item)), p = item's share of all `VIEW` events in the window
  (unseen items get the minimum p); higher = less popular items shown.
`GetPlacementMetrics(from, to, placement_id?, model_version?, request_id?)` returns the rows; with
`request_id` it returns that request's impression and click counts (the attribution proof). Admin only,
enforced in team-analytics and added to the gateway admin-only set (U11).

### D12 — Online comparison: A/B by user hash with a challenger

A new model family is trained on demand with `PUBLISH_MODE=challenger`; it passes the same offline gate and
sets `recs:v2:challenger` instead of `current`. An operator sets `recs:v2:challenger:share` (default 10, max
50) with `python -m recsys challenger start <mv> --share 10`. team-ai assigns bucket =
`int(sha256(challenger_mv + ":" + user_key)[:8], 16) % 100`; bucket < share ⇒ the challenger generation is
served (personal list, vectors and popular list all from it) and its `model_version` is returned. Sticky
per user per experiment; anonymous visitors bucket on `anon:<anonymous_id>`; no id ⇒ control. Nightly runs
of the control family keep promoting `current`. Decision is manual: compare family-level CTR in
`GetPlacementMetrics`, then `challenger promote` (challenger → current, current → previous) or
`challenger stop`. *Why A/B by hash:* it reuses the four PROGRAM attribution fields unchanged (`model_version`
identifies the arm), needs no merge logic in serving and no per-item arm field in events, and fails safe
(share 0). Shadow yields no exposure outcome; interleaving needs per-item team credit (not in the PROGRAM
event contract). Known bias: the challenger is a fixed generation while control refreshes nightly — keep
experiments ≤ 7 days (generation TTL) and read it as conservative for the challenger.

### D13 — Observability

team-ai `/metrics` adds: `recs_model_info{model_version,kind,arm}`, `recs_model_published_timestamp_seconds`,
`recs_last_run_status{status}`, `recs_last_run_finished_timestamp_seconds`,
`recs_requests_total{placement,source,arm}`, `recs_cache_requests_total{result}`,
`recs_eligibility_dropped_total`, `recs_eligibility_errors_total`, `recs_feature_failures_total`,
`recs_optout_requests_total`. Alert rules
`RecsModelStale` (age > 36h for 10m), `RecsRunFailing` (last status ≠ promoted for > 26h),
`RecsFallbackHigh` (popular+floor+empty share > 50 % for 30m), `RecsEligibilityErrors` (rate > 0 for 10m)
ship in `platform-core/infra` and `platform-gitops/platform/monitoring`. Dataset staleness surfaces as a
`dataset stale` refusal in the run report and therefore `RecsRunFailing`.

### D14 — Local development story

`scripts/recsys_local_pipeline.sh` (root), stack up: (1) `platform-e2e` traffic tool registers 2×N buyers,
seeds two categories of listings, and sends C1-shaped beacons through the gateway with each buyer's session
(history first, holdout last); (2) runs C2's documented local `analytics-ctl materialize --now` so the traffic is ingested and
materialized; (3) `docker compose --profile recsys run --rm recsys-train` with local thresholds, which triggers
the `als_interactions@v1` build itself (D1) and refuses if its watermark does not cover the traffic; (4) prints the run report and calls
`Recommend` on all three placements for two buyers. The e2e pipeline flow reuses the same steps.

## Dependencies on C1 / C2

| Work | Starts | Blocked on |
|---|---|---|
| Lifecycle (eval, gate, generations, pointer, lock, rollback, challenger), Qdrant/compose/gitops wiring, eligibility, placements with `NullFeatureClient`, filling `request_id`/`placement_id`, opt-out handling, metrics, alerts | now | C1 1.4 only (the serving proto fields; a small, first-landing C1 task). Trainer uses fixtures in the PROGRAM dataset layout |
| Input switch to real C2 dataset builds (trigger + wait), no-raw-events gate enforced in deploy, real `FeatureClient`, `similar_to_recent` / `covisit` / `trending`, NetworkPolicy to team-analytics | after C2 | C2 `BuildDataset`/`GetDatasetBuild` (5.5) + dataset client (12.2) + guard (12.3), `FeatureService` (6.1, 6.3), both scopes (4.1), the initial features |
| Frontend attribution, `GetPlacementMetrics`, attribution and opt-out e2e, online A/B readout | after C1 | C1 attribution fields stored by analytics, C1 SDK, `analytics.proto` edits (1.2), admin-set task (3.10), consent header (3.11) |

Program ownership (resolved): C1 owns the serving attribution proto fields and the admin-set task; C2 owns the
compose `DUCKDB_PATH` fix, the analytics PVC, the dataset client and the raw-read guard; this change owns the view
file, the trainer, serving, `listing_ids = 10` and `GetPlacementMetrics`. Full graph: `plans/ai-first/PROGRAM.md`.

## Failure modes

| Failure | Behaviour |
|---|---|
| Dataset build failed / timed out / stale / unknown schema / version mismatch | Run refused with reason; serving keeps current; `RecsRunFailing` |
| Gate refuses | Current untouched; first-ever run publishes a popularity generation |
| Job killed mid-load | Pointer not flipped; orphans removed by next cleanup or TTL |
| Redis down during flip | `EXEC` fails ⇒ `failed`; pointer unchanged |
| Qdrant down at serve time | ANN strategies ⇒ `[]` ⇒ next strategy; counted |
| FeatureService down / slow | Feature strategies ⇒ `[]`; `feature_failure`; trending keeps its last cache |
| team-search down | Empty items (fail closed); `RecsEligibilityErrors` |
| Challenger generation expired | Bucket users served by current; `challenger stop` recorded |
| Overlapping runs | Second run exits `skipped` |

## Risks / Trade-offs

- [A triggered build adds up to `DATASET_BUILD_TIMEOUT_SECONDS` to each run and competes for C2's heavy lane] →
  nightly cadence, one build per run, C2's queue of 4; a timeout refuses the run (current model stays).
  (C2's `fs-view.v1` snapshot spine takes an as-of list, so two snapshots fit in one view.)
- [Three bounded calls on the hot path] → each has its own timeout, trending is cached, budget documented.
- [Fail-closed hides rows when team-search is down] → accepted for privacy; alerted.
- [Tiny local data makes ALS and CTR noisy] → two separated tastes, local thresholds, relative assertions.
- [A/B volume is small locally] → the e2e asserts assignment and attribution, not significance.
- [Admin-only set grows] → pinned-test update is part of the task; reviewed by `auth-scope-reviewer`.
- [Key schema bump] → producer and consumer switch together.

## Migration Plan

1. C1 lands the serving attribution fields (C1 1.4); this change lands `listing_ids` (search) after it and
   `GetPlacementMetrics` (analytics) after C1 1.2 — backward compatible.
2. Land team-search `listing_ids`, team-ai v2 serving with placements and `NullFeatureClient`, platform-recsys
   v2 publish on fixtures; switch both to `v2` in one compose/gitops commit. v1 keys expire by TTL; the
   legacy `item_als_vectors` collection is deleted by the first v2 run.
3. After C2: point the trainer at the real dataset bucket, remove the raw drivers, enable the real
   `FeatureClient`; deploy CronJob + Qdrant + env in gitops; run the CronJob once by hand.
4. After C1: frontend attribution, `GetPlacementMetrics`, then A/B tooling in use.
5. Enable alert rules.

Change rollback: `RECS_ENABLED=false` on team-ai hides the rows; model rollback is `python -m recsys
rollback`; experiment rollback is `challenger stop`.

## Open Questions

- Exclude already-bought items from personal lists? Default no; decide after first online metrics.
- Gate against the current model on the same holdout (ADR-0016 follow-up)? Default later.
- Should C2 add a ranked "top items by feature" read (U12 alternative) for a fuller trending placement?
