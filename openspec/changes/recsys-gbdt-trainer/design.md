## Context

team-ai's `ranking.model: gbdt` is `GBDTRankerAdapter`: a fixed linear score over `[similarity, category_match, popularity,
price, freshness, ctr, cvr]` where price, freshness and cvr are 0 because no view provided them. It loads nothing. The
archived `wire-serving-gbdt-featurestore` change made serving read registry names and said the trainer must train on those same
features. `featurestore-item-attributes` adds `item_attributes@v1.price`. platform-recsys has no trainer, and no governed
dataset carries impressions (`als_interactions@v1` collapses events into per-pair weights).

## Goals / Non-Goals

**Goals:** a real learning-to-rank model whose inputs are values the online featurestore holds under the same names, trained and
evaluated without leakage, gated like ALS, shipped inside the generation in a format a dependency-free reader can score.

**Non-Goals:** serving integration (team-ai owner), user/context features, similarity as a feature (no offline value at
impression time), online learning.

## Decisions

### D1. Impressions come from a new governed dataset
`rank_training@v1` (featurestore): `user_key, impression_id, listing_id, position, label, occurred_at`. An impression is the
pair (`impression_id`, `listing_id`) of an `impression` event, the same attribution key `GetRecommendationPerformance` uses;
`label` is the best outcome carrying that key at or after the impression (add_to_cart 2, click 1, none 0). The dataset builder
takes per-dataset columns from the registry (`columns:` name -> `string|int|float|timestamp`); an entry without it keeps the
ALS columns, so `als_interactions@v1` and its lock hash are untouched.

### D2. One feature list, registry names, shared and tested
`recsys/ranker/contract.py::RANKING_FEATURES` = the seven `item_popularity@v1` features in registry order, then
`item_attributes@v1.price`, written `<view>.<feature>`:
`item_popularity.views_7d, clicks_7d, add_to_cart_7d, favorites_current, review_count, avg_rating, ctr_7d, item_attributes.price`.
Trees need no scaling, so the raw online values are the model's inputs (no derived popularity, no log). Missing or null = 0.0
(the serving defaults of `recommend/features.py`). `tests/test_ranker_contract.py` fails when the list drifts from
`platform-featurestore/registry/features.yaml` (names, order within each view, view versions) or from team-ai's
`RANKING_FEATURES` (read when team-ai is checked out, as team-ai's own contract test does for the registry). Similarity and
category match are not features: they have no offline value at impression time, and putting a constant there would be the
fake feature this contract exists to remove.

### D3. Point-in-time features
For each row the trainer picks the latest `as_of=*.parquet` of `item_popularity` (and `item_attributes`) with `as_of` not after
the row's `occurred_at`, so the features are what the online store could have served then. Rows with no earlier
`item_popularity` snapshot are dropped and counted (`rows_dropped_no_snapshot`); a missing attribute snapshot or row gives
price 0 (counted). The lineage records every snapshot file used with its SHA-256. Needs snapshots over time (the daily
materialisation keeps them: earlier runs are kept).

### D4. Debiased CTR and `ctr_source`
Serving replaces `ctr_7d` with the nearline position-debiased CTR when usable (`ctr_source` nearline/fallback). The trainer
mirrors it: from the TRAIN rows only, `clicks_ips / impressions` per item with clicks weighted by `position**0.5` (the nearline
estimator), capped at 1. A train row uses the value with its own contribution removed; a test row uses the full train value; with
fewer than `GBDT_CTR_MIN_IMPRESSIONS` impressions left the slot keeps the snapshot's `ctr_7d`. Each row records
`ctr_source` = `debiased` | `fallback`; the counts go to the model metadata and, with `GBDT_ROWS_PATH`, every row is written to
a Parquet file (features, label, position, impression id, `ctr_source`) for audit.

### D5. Temporal holdout, baseline and gate
Lists (impressions) are ordered by time; the last `GBDT_HOLDOUT_FRACTION` are the holdout (eval protocol
`temporal-lists-v1`), the rest train. As with ALS, evaluation trains on the split and the published model is refit on all rows;
the recorded metrics are the split model's. Metric: mean graded NDCG@10 (gain 2^label-1) over holdout lists with a positive.
Baseline: the incumbent fixed weights restricted to what the contract has, `0.15 * popularity + 0.10 * ctr` (popularity =
`log1p(views + 2 clicks + 5 add_to_cart + 3 favourites) / log1p(500)` capped at 1, the formula in team-ai), ties broken by a hash of
the listing id (not by shown position, which would reward the old ranking). Gate: no holdout positive -> rejected; `ndcg@10`
must exceed the baseline by `PROMOTION_MIN_RELATIVE_IMPROVEMENT`; then `ModelRegistry.evaluate` against the GBDT champion,
stored under `recs:model:champion:gbdt` (`ModelRegistry(champion_key=...)` sharing the store; the ALS champion is untouched).
`PROMOTION_FORCE` skips both. The candidate is registered as `gbdt-<model_version>` (type `gbdt`) and promoted only after the
publish succeeded, like ALS.

### D6. Trainer choice: a numpy LambdaMART, JSON model
LightGBM or XGBoost would add a native wheel (plus `libgomp`) to the recsys image, and team-ai would have to take the same
dependency, or a text-model parser, to load it; sklearn's gradient boosting has no ranking objective and a pickle format. The
data here is small (impressions of one window, 8 features) and the model is a few hundred nodes, so the trainer is ~150 lines of
numpy: histogram-binned regression trees (32 bins per feature, depth `GBDT_MAX_DEPTH`, Newton leaf values), LambdaRank
lambdas with |delta NDCG| over padded impression lists, `GBDT_TREES` rounds at `GBDT_LEARNING_RATE`. Deterministic (no
sampling). Impression lists are truncated to `GBDT_MAX_LIST` by position and the number of lists capped at `GBDT_MAX_LISTS`
(most recent kept) to bound memory. No image or dependency change; the artifact is plain JSON any service scores in pure Python.

### D7. Artifact format and where it lives (for the team-ai owner)
Key `recs:v1:gen:<generation>:ranker` (a STRING, TTL = the generation's), `<generation>` = the value of `recs:v1:serving`.
Written by `publish_generation` before the pointer moves; pruned and TTL-refreshed with the generation (the generation key
pattern now includes `ranker$`). Absent when the stage is off or the candidate was rejected.

```json
{"format": "agora-gbdt/1", "objective": "lambdarank", "model_version": "gbdt-<generation>", "generation": "<generation>",
 "features": ["item_popularity.views_7d", "...", "item_attributes.price"],
 "feature_views": {"item_popularity": 1, "item_attributes": 1},
 "defaults": {"<feature>": 0.0},
 "ctr_feature": "item_popularity.ctr_7d",
 "base_score": 0.0, "learning_rate": 0.1,
 "trees": [{"feature": [3, -1, -1], "threshold": [12.5, 0.0, 0.0], "left": [1, -1, -1], "right": [2, -1, -1], "value": [0.0, -0.4, 0.7]}],
 "metrics": {"ndcg@10": 0.0, "baseline_ndcg@10": 0.0, "eval_protocol": "temporal-lists-v1"}, "trained_at": "<RFC 3339>"}
```
Node `i` of a tree is internal when `feature[i] >= 0`: go to `left[i]` if `x[feature[i]] <= threshold[i]`, else `right[i]`;
a leaf has `feature[i] = -1` and score `value[i]`. Root is node 0. `score(x) = base_score + learning_rate * sum(leaf values)`;
higher ranks first. `x` has one entry per name in `features`, in order.

**Loader contract (not implemented here, team-ai).** Per request: read `recs:v1:serving`, then the `ranker` key of that
generation (cache by generation name; on any miss, parse error, `format` other than `agora-gbdt/1`, or a `features` list that is
not equal to team-ai's `RANKING_FEATURES`, keep the built-in weights and count the fallback). Build `x` per candidate from
`fs:item_popularity:v<N>:<listing_id>` (N from `fs:item_popularity:current`) and `fs:item_attributes:v<N>:<listing_id>` (N from
`fs:item_attributes:current`): value of `<view>.<feature>`, 0.0 when the row, key or value is missing, null or not finite;
for `item_popularity.ctr_7d` use the nearline debiased CTR when usable (that is what `ctr_source` means in training), else
`ctr_7d`. Score with the loop above in pure Python (no new dependency) and sort descending; similarity from retrieval can stay
the tie-break. `RedisFeatureStore` needs a second view reader for `item_attributes`; `GBDTRankerAdapter` needs a
`from_artifact` constructor beside the weights one. Nothing else in serving changes.

## Risks / Trade-offs
- Item-only features cannot beat a good item CTR by much; the model's value grows with the attribute views and, later, user
  features. The gate compares with the incumbent weights on the same rows so it cannot regress silently.
- The training CTR is over the dataset window, the serving one over 24 h (nearline): same estimator and clamp, different
  horizon. If it shifts rankings, set `GBDT_CTR_MIN_IMPRESSIONS` high so `fallback` dominates.
- Point-in-time features need snapshots older than the impressions; with a single fresh snapshot every row is dropped and the
  stage rejects the candidate ("no usable rows").
- A rejected candidate ships a generation without a ranker (serving keeps its built-in weights) instead of inheriting the previous generation's model, which was trained on older features.
