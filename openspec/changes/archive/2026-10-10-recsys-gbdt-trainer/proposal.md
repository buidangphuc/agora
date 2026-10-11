## Why

team-ai serves `ranking.model: gbdt`, but the "GBDT" is a fixed linear weight vector with a synergy bonus
(`GBDTRankerAdapter`, `[0.30, 0.25, 0.15, 0.05, 0.05, 0.10, 0.10]`); no model has ever been trained. platform-recsys only has
the same fixed weights in `recsys/ranker/` (no trainer, no data), and the archived `wire-serving-gbdt-featurestore` change
recorded that the trainer feature space (10 ad-hoc features) is not the serving one (registry names). With
`featurestore-item-attributes` the registry now has price, so a real contract can be trained and served: this change adds
the learning-to-rank trainer and ships its model as part of the generation.

## What Changes

- **platform-featurestore**: a governed dataset `rank_training@v1` (one row per item impression, labelled click / add-to-cart
  / none from events carrying the same `impression_id`), built point in time like `als_interactions@v1`. The dataset builder
  learns per-dataset columns (the ALS dataset is unchanged).
- **platform-recsys**: a GBDT trainer in the batch run (`ENABLE_GBDT`): LambdaRank gradient boosting over the registry
  features in one explicit order (`recsys/ranker/contract.py`, parity-tested against `features.yaml` and team-ai's list), on
  the `rank_training@v1` rows joined point in time to the featurestore offline snapshots, with the debiased CTR source
  recorded per training row (`ctr_source`), evaluated on a temporal holdout against the fixed-weight baseline, gated like the
  ALS model (its own champion), and published as `recs:v1:gen:<generation>:ranker` (portable JSON) before the pointer moves.
  Retention and TTL refresh cover the new key.
- **team-ai** (feature list only): `RANKING_FEATURES` in `recommend/features.py`, the shared list the trainer's contract is
  tested against. The loader that reads the artifact is described in `design.md` for the team-ai owner.

## Capabilities

### New Capabilities
- `gbdt-ranking-training`: the trainer, its inputs, evaluation, gate and artifact.

### Modified Capabilities
- `governed-datasets`: the ranking training dataset.
- `recsys-generations`: the ranker artifact is part of the generation.

## Impact

- platform-featurestore: `registry/` (dataset + SQL + lock), `featurestore/dataset.py`, `registry.py`, tests, README.
- platform-recsys: `recsys/ranker/*`, `config.py` + `.env.example`, `pipeline.py`, `publish.py`, `load/redis_cache.py`,
  `registry/registry.py`, tests, README. No new dependency (numpy, pandas, pyarrow are in the image); no image size change.
- team-ai: one constant in `app/modules/business/recommend/features.py`.
- Images to rebuild: platform-featurestore, platform-recsys.

## Non-goals

- No change to team-ai's ranker or loader (described, not edited); no serving-side blending rules.
- No user features, similarity or category match in the model (no offline source at impression time).
- No online learning; no change to ALS, the two-tower stage or the gateway.
